# API 设计（v1）

| 项 | 内容 |
| --- | --- |
| Base URL | `/api/v1` |
| 传输 | JSON（`Content-Type: application/json`）；文件上传 `multipart/form-data` |
| 字符集 | UTF-8 |
| 鉴权 | `Authorization: Bearer <jwt>`，有效期 30 天 |
| 时间 | RFC3339 UTC，如 `2026-10-27T08:30:00Z` |

---

## 1. 通用约定

### 1.1 统一响应

成功：

```json
{ "code": 0, "message": "ok", "data": { } }
```

失败：

```json
{ "code": 1002, "message": "登录已过期，请重新登录", "data": null }
```

- HTTP 状态码表达语义层级（400/401/403/404/409/422/500），`code` 表达具体业务原因。
- 校验失败时 `data` 返回字段级错误：`{"fields":{"username":"用户名不能为空"}}`。

### 1.2 错误码

| code | HTTP | 含义 |
| --- | --- | --- |
| 0 | 200 | 成功 |
| 1001 | 400 | 参数错误 |
| 1002 | 401 | 未登录 / 令牌无效或过期 |
| 1003 | 403 | 无权限（角色或资源归属） |
| 1004 | 404 | 资源不存在 |
| 1005 | 409 | 冲突（用户名已存在、重复提交、状态冲突） |
| 1006 | 422 | 业务规则不允许（试卷未发布、试卷缺答案等） |
| 1007 | 413/415 | 文件过大 / 类型不支持 |
| 2001 | 502 | AI 服务不可用（连接失败/超时） |
| 2002 | 502 | AI 返回内容无法解析 |
| 2003 | 409 | 未配置可用的 AI 模型 |
| 2004 | 429 | AI 限流，请稍后重试 |
| 3001 | 500 | 服务器内部错误 |

### 1.3 分页

请求：`?page=1&page_size=20`（默认 1 / 20，最大 100）。

响应 `data`：

```json
{ "items": [], "total": 0, "page": 1, "page_size": 20 }
```

### 1.4 鉴权与角色

- 除 `POST /auth/login`、`GET /healthz`、`GET /version` 外，全部需要 Bearer Token。
- JWT claims：

```json
{ "sub": "12", "role": "teacher", "name": "王老师", "ver": 0, "iat": 1793000000, "exp": 1795592000 }
```

- 服务端校验：签名、`exp`、用户存在且 `status=1`、`ver == users.token_version`。
- 角色不符返回 403/1003。

### 1.5 限流

| 接口 | 限制 |
| --- | --- |
| `POST /auth/login` | 10 次/分钟/IP，同账号连续失败 5 次锁 10 分钟 |
| `POST /files` | 30 次/分钟/用户 |
| `POST /papers/{id}/parse` | 每试卷 5 次/小时 |
| `POST /student/attempts/{id}/submit` | 5 次/分钟/用户 |

---

## 2. 认证

### POST `/auth/login`

```json
{ "username": "stu001", "password": "Init@1234" }
```

200：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "eyJhbGciOi...",
    "expires_at": "2026-11-26T08:30:00Z",
    "user": { "id": 12, "username": "stu001", "display_name": "张三", "role": "student", "class_name": "初二3班", "grade": "初二", "must_change_pwd": true }
  }
}
```

错误：1002（账号或密码错误，统一文案，避免枚举）、1003（账号已禁用）。

### GET `/auth/me`

200 `data`：同 `user` 结构（不含密码），附 `provider_summary`（老师端：AI 是否已配置）。

### POST `/auth/change-password`

```json
{ "old_password": "Init@1234", "new_password": "Abc@2026" }
```

200；旧 token 因 `token_version+1` 失效，返回新 token：

```json
{ "code": 0, "message": "ok", "data": { "token": "..." } }
```

规则：长度 8-64，至少含字母与数字；不得与旧密码相同。

---

## 3. 学生管理（老师）

### GET `/students?keyword=&class_name=&status=&page=&page_size=`

200：

```json
{
  "code": 0, "message": "ok",
  "data": {
    "items": [
      { "id": 12, "username": "stu001", "display_name": "张三", "class_name": "初二3班", "grade": "初二", "status": 1, "last_login_at": "2026-10-20T10:00:00Z", "created_at": "2026-09-01T02:00:00Z" }
    ],
    "total": 1, "page": 1, "page_size": 20
  }
}
```

### POST `/students`

```json
{ "username": "stu002", "display_name": "李四", "password": "Init@1234", "class_name": "初二3班", "grade": "初二" }
```

201 `data`：创建后的学生（`password` 不回显）。`password` 省略时服务端生成 10 位随机密码并在 `data.initial_password` 返回一次。

校验：用户名非空、≤64 字符（支持中文、字母、数字和符号），全局唯一；姓名 1-32。

### PUT `/students/{id}`

```json
{ "display_name": "李四", "class_name": "初二4班", "grade": "初二", "status": 1 }
```

200；`status=0` 时同时 `token_version+1`（强制下线）。

### POST `/students/{id}/reset-password`

```json
{ "new_password": "Init@5678" }
```

`new_password` 省略则生成随机值返回 `data.initial_password`；同时 `token_version+1`。

### DELETE `/students/{id}`

204（软删除，`deleted_at` 置位，`token_version+1`）。存在进行中答卷时返回 1006 并提示先处理。

---

## 4. 科目

### GET `/subjects?stage=&status=`

200：

```json
{
  "code": 0, "message": "ok",
  "data": { "items": [ { "id": 2, "stage": "junior", "name": "数学", "code": "math", "sort_order": 10, "status": 1, "paper_count": 4 } ] }
}
```

### POST `/subjects`

```json
{ "stage": "junior", "name": "数学", "code": "math", "sort_order": 10 }
```

201；`(stage,name)` 重复返回 1005。

### PUT `/subjects/{id}` / DELETE `/subjects/{id}`

PUT 可改名、排序、启停。DELETE 若存在引用试卷返回 1006（提示改为禁用）。

---

## 5. AI 模型配置（老师）

### GET `/ai/providers`

200：

```json
{
  "code": 0, "message": "ok",
  "data": {
    "items": [
      {
        "id": 1, "name": "DeepSeek V3", "base_url": "https://api.deepseek.com/v1",
        "model": "deepseek-chat", "api_key_hint": "****ab12",
        "supports_vision": false,
        "temperature": 0.2, "timeout_sec": 120, "concurrency": 2,
        "thinking_mode": "",
        "status": 1, "roles": ["grading", "chat"], "created_at": "2026-10-01T00:00:00Z"
      }
    ],
    "roles": { "vision": 2, "grading": 1, "chat": 1, "default": 1 }
  }
}
```

### POST `/ai/providers`

```json
{
  "name": "通义千问 VL", "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1",
  "api_key": "sk-xxx", "model": "qwen-vl-max",
  "supports_vision": true,
  "temperature": 0.2, "timeout_sec": 180, "concurrency": 2,
  "thinking_mode": "high",
  "extra_headers": {}, "extra_body": {}
}
```

> `thinking_mode` 取值：`""`（服务商默认，不传）、`none`（关闭）、`low`、`high`、`max`；对应请求体的 `reasoning_effort`。
>
> 对 `opencode.ai` 主机，服务端会自动附加 `User-Agent: onlineTest/0.1` 与稳定的 `x-opencode-session`（按 provider 派生）；`extra_headers` 中的同名头优先。

201：返回脱敏对象；`api_key` 不入库明文。若传 `"set_default_vision": true` 等字段，同步写角色绑定。

### PUT `/ai/providers/{id}`

字段同 POST，`api_key` 省略表示不变；传空字符串表示清空。

### DELETE `/ai/providers/{id}`

204；被角色绑定的 provider 删除时自动解绑并回落到 `ai.default`，响应中给出提示。

> `max_tokens` 与 `supports_json` 已废弃：生成长度使用服务商默认值，结构化输出始终请求 JSON 模式；旧字段仍可解析但会被忽略。

### POST `/ai/providers/{id}/test`

```json
{ "probe_vision": false }
```

200：

```json
{ "code": 0, "message": "ok", "data": { "ok": true, "latency_ms": 812, "model": "deepseek-chat", "reply": "pong", "vision_ok": null } }
```

失败返回 2001/2002，`message` 含可读原因（401 密钥无效、404 路径错误、超时等）。

### POST `/ai/roles`

```json
{ "role": "vision", "provider_id": 2 }
```

`role ∈ {vision, grading, chat, default}`；200 返回最新绑定表。

### GET `/ai/providers/{id}/health`（P1）

近 24 小时成功率、平均延迟、失败原因分布。

### GET `/ai/calls?purpose=&status=&page=&page_size=`

调用日志（`ai_calls`），不含 prompt 明文，仅摘要与用量。

---

## 6. 文件

### POST `/files`

`multipart/form-data`：

| 字段 | 说明 |
| --- | --- |
| `file` | 单文件，≤20MB，jpeg/png/webp/pdf |
| `kind` | `paper_source`（默认）/`question_image` |

201：

```json
{ "code": 0, "message": "ok", "data": { "id": "8f1c...", "original_name": "page1.jpg", "mime": "image/jpeg", "size_bytes": 842113, "width": 1600, "height": 2263, "kind": "paper_source" } }
```

错误：1007（类型/大小）、1001（缺文件）。

### GET `/files/{id}`

- 学生仅可访问自己作答试卷的关联文件；老师可访问全部。
- 响应头：`Content-Type` 取库中 mime、`X-Content-Type-Options: nosniff`、`Content-Disposition: inline; filename*=UTF-8''...`。
- 支持 `?download=1` 强制下载。

---

## 7. 试卷（老师）

### GET `/papers?subject_id=&stage=&status=&keyword=&page=&page_size=`

200：

```json
{
  "code": 0, "message": "ok",
  "data": {
    "items": [
      {
        "id": 7, "title": "初二数学期中试卷", "subject_id": 2, "subject_name": "数学",
        "stage": "junior", "grade": "初二", "status": "published",
        "question_count": 23, "total_score": 100, "duration_min": 120,
        "parse_status": "succeeded", "source_type": "image",
        "published_at": "2026-10-20T01:00:00Z", "attempt_count": 31, "graded_count": 28
      }
    ],
    "total": 1, "page": 1, "page_size": 20
  }
}
```

### POST `/papers`

```json
{ "title": "初二数学期中试卷", "subject_id": 2, "stage": "junior", "grade": "初二", "duration_min": 120, "description": "", "ai_grading": true, "show_answer": true }
```

201 返回试卷详情（`status=draft, parse_status=none`）。

### GET `/papers/{id}`

返回试卷完整结构（老师视角），含 `questions[]` 与 `files[]`：

```json
{
  "code": 0, "message": "ok",
  "data": {
    "id": 7, "title": "初二数学期中试卷", "stage": "junior", "grade": "初二",
    "subject": { "id": 2, "name": "数学" },
    "status": "ready", "parse_status": "succeeded", "parse_error": "",
    "question_count": 23, "total_score": 100, "duration_min": 120,
    "ai_grading": true, "show_answer": true,
    "files": [ { "id": "8f1c...", "role": "source", "page_no": 0, "mime": "image/jpeg", "url": "/api/v1/files/8f1c..." } ],
    "questions": [
      {
        "id": 101, "seq": 1, "type": "single",
        "stem": "若 $x^2-5x+6=0$，则 $x$ 的值为（ ）",
        "options": [ { "key": "A", "text": "2 或 3" }, { "key": "B", "text": "-2 或 -3" } ],
        "answer": ["A"], "score": 3, "analysis": "因式分解得 $(x-2)(x-3)=0$",
        "knowledge": ["一元二次方程"], "difficulty": "easy",
        "ai_confidence": 0.92, "need_manual": false, "grading_mode": "auto"
      }
    ],
    "stats": { "attempts": 31, "graded": 28, "avg_score": 78.4 }
  }
}
```

### PUT `/papers/{id}`

可改：`title`、`subject_id`、`stage`、`grade`、`description`、`duration_min`、`ai_grading`、`show_answer`。
已发布试卷改 `subject_id`/`stage` 返回 1006；其他字段允许并提示影响面（`data.affected_attempts`）。

### DELETE `/papers/{id}`

- 无作答记录：204 物理删除（`questions` 级联）。
- 有作答记录：1006，建议 `PATCH status=archived`。

### POST `/papers/{id}/publish`

无请求体。校验：至少 1 题、`total_score > 0`、每题分值 > 0、非 `manual` 题必须有 `answer`（`need_manual=1` 的题必须为 `manual` 或补答案）。

200 `data`：`{ "status": "published", "published_at": "..." }`；失败 1006 + `data.problems[]`。

发布只做校验（题目数、分值、答案、选项、人工阅卷标记），不调用 AI；缺答案的题目应在解析/校对阶段补齐。

### POST `/papers/{id}/archive` · POST `/papers/{id}/restore`

归档 / 取消归档。

### 解析上传

#### POST `/papers/{id}/parse`

`multipart/form-data`：

| 字段 | 说明 |
| --- | --- |
| `files` | 1-30 个图片或 PDF（单文件 ≤20MB，单次累计 ≤200MB） |
| `provider_id` | 可选，指定识别模型；缺省用 `ai.role.vision` |
| `hint` | 可选，如"共 25 题，含 8 道解答题" |
| `mode` | `replace`（默认，替换文件并清空已有题目）/ `append`（仅识别本次新文件，保留已有文件与题目，题号追加） |

202：

```json
{ "code": 0, "message": "ok", "data": { "job_id": "b7d1...", "paper_id": 7, "parse_status": "queued" } }
```

校验：`status ∈ {draft, ready, published（仅 append）}`；PDF 无文字层时不用 400，任务直接失败并给引导（见任务 message）。同一试卷存在活动任务返回 1005 + 现有 `job_id`。

增量上传说明：

- `append` 可反复调用：每次只把本次文件追加到 `paper_files`（页码顺延），解析任务只读取本次 `file_ids`，新题目按现有最大题号继续追加，`question_count`/`total_score` 重新汇总。
- `replace` 会删除该试卷原有文件关联，并在解析成功后清空原有题目后重建。
- 已发布试卷仅允许 `append` 追加题目并保持发布状态，不允许 `replace`；已归档试卷不能上传；同一试卷同一时间只允许一个解析任务。
- 文件按 SHA256 去重：`append` 时同一文件已存在则跳过；若本次全部文件重复，返回 `409 / 1005`，不创建任务。
- 题目按「题型 + 规范化题干 + 选项」签名去重（选项顺序不敏感）；重复题不新增、题号不占位，任务 message 包含「跳过重复题 N 道」。
- 解析入库后若存在缺失答案，任务会自动调用 AI（`ai.role.grading`，8 题一批）补全答案与解析，进度 message 显示「正在解析缺失答案」；成功时最终 message 包含「AI 已补全 N 道答案」。
- 解析成功后自动删除本次上传的原始文件、`files` 记录与 `paper_files` 关联（message 包含「原始文件已清理」），试卷详情不再展示原始文件；解析失败时保留文件，允许重试或重新上传。
- 补答失败或部分题目无法解析不会判定识别失败：题目保留、`parse_status=succeeded`，`parse_error` 写入可读警告，需老师在校对页补充；发布校验会再次拦截。

#### POST `/papers/{id}/questions`

```json
{ "type": "single", "stem": "……", "options": [{"key":"A","text":"…"}], "answer": ["A"], "score": 3, "analysis": "", "knowledge": ["…"], "difficulty": "medium", "grading_mode": "auto" }
```

201；`seq` 由服务端追加到末尾。

#### PUT `/papers/{id}/questions/{qid}`

字段同 POST（局部更新）。

#### DELETE `/papers/{id}/questions/{qid}`

204；服务端重排 `seq` 并同步统计。

#### POST `/papers/{id}/questions/{qid}/ai-answer`

调用 `ai.role.grading` 独立解答该题，并复核已有答案：

- 答案缺失：补全正确答案、解析与置信度；
- 已有答案正确：保留原答案，仅刷新复核反馈/置信度；
- 已有答案错误：改为 AI 给出的正确答案；
- AI 无法给出有效答案：保留原答案并标记 `need_manual=true`。

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "question": { "id": 7, "answer": ["C"], "need_manual": false, "...": "..." },
    "changed": true,
    "feedback": "原答案缺失，补充正确答案 C",
    "previous_answer": []
  }
}
```

单题请求通常数秒完成；前端应在请求期间禁用按钮并提示「AI 正在作答」。未配置可用模型或调用失败返回 `502 / 2001`。

#### PUT `/papers/{id}/questions/order`

```json
{ "question_ids": [103, 101, 102] }
```

200，返回重排后的 `seq` 列表。

---

## 8. 学生端

### GET `/student/papers?subject_id=&page=&page_size=`

仅返回 `status=published` 的试卷，附个人状态：

```json
{
  "code": 0, "message": "ok",
  "data": {
    "items": [
      {
        "id": 7, "title": "初二数学期中试卷", "subject_name": "数学", "grade": "初二",
        "question_count": 23, "total_score": 100, "duration_min": 120,
        "published_at": "2026-10-20T01:00:00Z",
        "my_attempt": { "id": 55, "status": "graded", "score": 86, "submitted_at": "2026-10-21T09:00:00Z" }
      }
    ],
    "total": 1, "page": 1, "page_size": 20
  }
}
```

### POST `/student/papers/{paperId}/attempts`

开始作答（幂等）：

- 已存在 `in_progress` → 返回该 attempt（200）。
- 已存在 `graded` 且 `allow_retry=0` → 1006。
- 否则创建 `attempt_no = 历史最大 + 1`，返回 201。

200/201：

```json
{
  "code": 0, "message": "ok",
  "data": {
    "attempt": { "id": 55, "paper_id": 7, "status": "in_progress", "mode": "exam", "started_at": "2026-10-27T02:00:00Z", "duration_sec": 0, "server_time": "2026-10-27T02:00:00Z" },
    "paper": { "id": 7, "title": "初二数学期中试卷", "duration_min": 120, "total_score": 100, "show_answer": true },
    "questions": [ { "id": 101, "seq": 1, "type": "single", "stem": "…", "options": [], "score": 3 } ],
    "answers": [ { "question_id": 101, "answer": [] } ]
  }
}
```

> 作答期间不下发 `answer`/`analysis`；练习模式调用单题核对接口后才返回。

### GET `/student/attempts/{id}`

返回当前答卷快照（刷新恢复）：`attempt`（含剩余时间与 `mode`）、`paper`、`questions`（无答案）、`answers`、`saved_at`。

### PUT `/student/attempts/{id}/mode`

切换作答模式（仅 `in_progress`）：

```json
{ "mode": "practice" }
```

`mode` 取值：

- `exam`：统一交卷，交卷前不显示答案；
- `practice`：边做边看答案，允许逐题核对。

200：

```json
{ "code": 0, "message": "ok", "data": { "mode": "practice" } }
```

### POST `/student/attempts/{id}/questions/{qid}/check`

练习模式下核对单题并返回答案与解析（仅 `in_progress`，且 `mode=practice`）：

```json
{ "answer": ["A"] }
```

作答非空；服务端会先保存该题答案，再做本地判定。客观题返回对错与得分；解答题返回参考答案与解析，不做自动判分。

200：

```json
{
  "code": 0, "message": "ok",
  "data": {
    "question_id": 101,
    "type": "single",
    "answer": ["A"],
    "correct_answer": ["C"],
    "is_correct": false,
    "graded": true,
    "score": 0,
    "max_score": 3,
    "analysis": "……",
    "feedback": "回答错误"
  }
}
```

- `is_correct` 为 `null`、`graded=false`、`score=null` 时表示主观题，仅展示参考答案。
- 本题核对不会写入最终成绩；交卷时仍按正式流程统一判分。

### PUT `/student/attempts/{id}/answers`

批量幂等保存（只允许 `in_progress`）：

```json
{
  "answers": [
    { "question_id": 101, "answer": ["A"] },
    { "question_id": 105, "answer": ["\\frac{1}{2}", "x>1"] },
    { "question_id": 112, "answer": ["解：由题意得\n$$x^2-5x+6=0$$\n因式分解得 $(x-2)(x-3)=0$，所以\n$$x_1=2,\\ x_2=3$$"] }
  ]
}
```

200：

```json
{ "code": 0, "message": "ok", "data": { "saved": 2, "saved_at": "2026-10-27T02:03:11Z", "remaining_sec": 7089 } }
```

规则：单题答案 ≤16KB（其中 LaTeX 源码 ≤4KB）；题号必须属于该试卷；超出时限（`duration_min>0` 且超时 5 分钟）返回 1006。

答案内容约定（详见数据库设计 §6.4）：

- 客观题存选项 key / `true|false` / 空位文本数组。
- 数学填空存 LaTeX，如 `["\\frac{1}{2}"]`；多空按顺序。
- 解答题为**单个字符串**，文本与公式混排：行内 `$latex$`、独立公式 `$$latex$$`、换行 `\n`、字面量 `$` 转义为 `\$`。
- 服务端只接受纯文本（JSON 字符串），不接收 HTML；长度超限返回 1001。
- 保存接口不做公式语法校验（避免阻断作答），前端 MathLive 保证输出合法 LaTeX；渲染失败时降级为纯文本。

### POST `/student/attempts/{id}/submit`

```json
{ "confirm": true }
```

200：

```json
{ "code": 0, "message": "ok", "data": { "status": "grading", "objective_score": 45, "pending_questions": 3, "job_id": "c9a2..." } }
```

- 无主观题或 `paper.ai_grading=0`：`status=graded`，返回 `score`。
- 已提交：返回当前状态（幂等，不重复判分）。

### GET `/student/attempts/{id}/result`

```json
{
  "code": 0, "message": "ok",
  "data": {
    "attempt": {
      "id": 55, "status": "graded", "score": 86, "full_score": 100,
      "objective_score": 45, "subjective_score": 41, "correct_count": 18,
      "duration_sec": 3560, "submitted_at": "2026-10-27T02:59:00Z", "graded_at": "2026-10-27T03:00:12Z",
      "grade_error": ""
    },
    "paper": { "id": 7, "title": "初二数学期中试卷", "show_answer": true },
    "questions": [
      {
        "id": 101, "seq": 1, "type": "single", "stem": "…", "options": [],
        "score": 3, "max_score": 3, "my_answer": ["A"], "correct_answer": ["A"],
        "is_correct": true, "grading_mode": "local", "feedback": "", "missing": [], "advice": "",
        "analysis": "因式分解得 …"
      },
      {
        "id": 112, "seq": 12, "type": "short", "stem": "…",
        "score": 6, "max_score": 10,
        "my_answer": ["解：由题意得\n$$x^2-5x+6=0$$\n因式分解得 $(x-2)(x-3)=0$，所以 $x_1=2,\\ x_2=3$"],
        "correct_answer": null,
        "is_correct": null, "grading_mode": "ai",
        "feedback": "思路正确，未讨论 $a=0$ 的情形。",
        "missing": ["参数讨论", "结论完整性"], "advice": "分类讨论时先列全条件。"
      }
    ],
    "report": { "accuracy": 0.72, "score_rate": 0.86, "summary": "…", "weak_points": [], "suggestions": [] }
  }
}
```

`status != graded` 时 `questions` 仅返回已判部分，`report` 为空对象；`show_answer=false` 时 `correct_answer=null`。

### GET `/student/attempts?paper_id=&status=&page=&page_size=`

历史列表：试卷标题、状态、得分、提交时间。

---

## 9. 任务

### GET `/jobs/{id}`

```json
{
  "code": 0, "message": "ok",
  "data": {
    "id": "b7d1...", "type": "paper_parse", "ref_type": "paper", "ref_id": 7,
    "status": "running", "progress": 40, "message": "已识别 8/20 页，解析出 15 题",
    "attempts": 0, "last_error": "",
    "created_at": "2026-10-27T02:00:00Z", "started_at": "2026-10-27T02:00:01Z", "finished_at": null
  }
}
```

`status`：`pending | running | succeeded | failed | canceled`。失败时 `last_error` 为可读原因（如"扫描版 PDF 无可提取文字，请上传页面截图"）。建议 2s 轮询，`succeeded/failed` 后停止。

### POST `/jobs/{id}/retry`

仅 `failed` 可重试，创建新任务并返回 `job_id`（同 ref 保留历史，不覆盖）。`type=paper_parse` 时若旧任务已部分入库，`mode=replace` 重新开始。

### POST `/jobs/{id}/cancel`

仅 `pending/running`；worker 在下一个检查点退出，已产生数据保留（解析任务会清空草稿题，见 `mode`）。

### GET `/jobs?type=&status=&page=&page_size=`

老师端任务列表（近 7 天）。

---

## 10. 成绩（老师端）

### GET `/teacher/attempts?paper_id=&student_id=&status=&page=&page_size=`

```json
{
  "code": 0, "message": "ok",
  "data": {
    "items": [
      { "id": 55, "paper_id": 7, "paper_title": "初二数学期中试卷", "student_id": 12, "student_name": "张三", "class_name": "初二3班", "status": "graded", "score": 86, "full_score": 100, "submitted_at": "2026-10-27T02:59:00Z" }
    ],
    "total": 1, "page": 1, "page_size": 20,
    "summary": { "submitted": 1, "graded": 1, "avg_score": 86, "max_score": 86, "min_score": 86 }
  }
}
```

### GET `/teacher/attempts/{id}/result`

同 `/student/attempts/{id}/result`，额外返回 `student` 与 `manual_review[]`（待人工复核题目）。

### PUT `/teacher/attempts/{id}/answers/{questionId}`

人工改分（P1）：

```json
{ "score": 8, "feedback": "步骤分补齐", "regenerate_report": true }
```

200 返回新的汇总分；写 `audit_logs`。

### POST `/teacher/attempts/{id}/regrade`

重新阅卷（P1）：重新调用 AI 或仅重算汇总，`{ "only_ungraded": true }`。

---

## 11. 系统

### GET `/healthz`

无需鉴权：

```json
{ "code": 0, "message": "ok", "data": { "ok": true, "db": "ok", "queue": { "pending": 1, "running": 1 }, "version": "0.1.0", "uptime_sec": 3600 } }
```

### GET `/version`

```json
{ "code": 0, "message": "ok", "data": { "version": "0.1.0", "commit": "dev", "go": "go1.26.4", "built_at": "2026-10-27T00:00:00Z" } }
```

### GET `/stats/overview`（老师，P1）

```json
{ "code": 0, "message": "ok", "data": { "students": 42, "papers": 8, "published": 5, "attempts_today": 12, "pending_jobs": 0, "ai_calls_today": 36, "ai_tokens_today": 184320 } }
```

---

## 12. 典型时序

### 12.1 上传识别

```
老师 → POST /files ×N            （拿 file id，或直接走 /papers/{id}/parse 一步上传）
老师 → POST /papers               （创建草稿，拿 paper_id）
老师 → POST /papers/{id}/parse    （multipart，返回 job_id）
前端 → GET /jobs/{job_id} 每 2s   （progress / message）
Worker → AI 识别 → 写 questions  → 更新 parse_status
前端 → GET /papers/{id}           （校对页）
老师 → PUT /papers/{id}/questions/{qid} ×N（修正）
老师 → POST /papers/{id}/publish
```

### 12.2 学生答题与阅卷

```
学生 → POST /student/papers/{id}/attempts   （拿 attempt + questions）
前端 → PUT  /student/attempts/{id}/mode      （可选：exam / practice）
前端 → PUT  /student/attempts/{id}/answers   （2s 防抖，逐批）
练习模式 → POST /student/attempts/{id}/questions/{qid}/check（单题核对）
学生 → POST /student/attempts/{id}/submit    （客观题即时判分，返回 job_id）
前端 → GET  /student/attempts/{id}/result    （每 3s 轮询直到 graded）
Worker → 批量 AI 阅卷 → 汇总 → 生成报告 → status=graded
```

---

## 13. 实现约定（后端）

1. Handler 签名统一 `func(w http.ResponseWriter, r *http.Request)`，通过 `app` 结构体注入依赖，避免全局变量。
2. 参数解析统一 `decodeJSON(r, &req, maxBytes)`，拒绝未知字段（`DisallowUnknownFields`），返回字段级错误。
3. 所有需要归属校验的接口集中在 Service 层 `assertOwnership(actor, resource)`，Handler 不写权限逻辑。
4. 分页 helper `parsePage(r)`；排序字段白名单在 Store 层。
5. 写接口返回 201/204 语义明确；幂等接口（保存答案、开始答卷、提交）重复调用返回同一结果。
6. 错误包装：`fmt.Errorf("parse paper %d: %w", id, err)`，Handler 用 `errors.As` 提取 `AppError`。
7. 上传接口在读取前检查 `Content-Length` 并 `http.MaxBytesReader` 兜底。
8. 数据访问统一走 GORM（Repository），Handler/Service 不直接持有 `*gorm.DB`；条件查询用 `Where("x = ?", v)` 参数绑定，`Raw` 仅限内部统计且必须带 `?`，`Order()` 用白名单映射；多表写用 `db.Transaction`，错误经 `mapDBError` 转 `AppError`。

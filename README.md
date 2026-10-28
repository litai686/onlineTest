# 在线做题系统

老师上传试卷（图片 / PDF）→ AI 识别成结构化题目 → 学生在线作答 → 客观题本地判分 + 主观题 AI 阅卷 → 生成逐题评语与整卷学习建议。

- 后端：Go 1.26 + 标准库 `net/http` + GORM + SQLite（`glebarez/sqlite`，纯 Go，无 CGO）
- 前端：原生 HTML / CSS / ES Modules，零框架、零构建，`go:embed` 打进二进制
- 认证：JWT（默认 30 天），账号由老师创建，学生无需注册
- 公式：KaTeX 渲染 + MathLive 公式编辑（本地 vendor，离线可用）
- 网关兼容：OpenCode Zen/Go 自动附加 `x-opencode-session` 与自有 `User-Agent`，也支持在模型配置里填写自定义请求头
- AI：任意 OpenAI 兼容接口（DeepSeek / 通义 / OpenAI / Ollama / vLLM 等）

## 快速开始

```bash
# 1. 编译（Windows/Linux 均可，产物单文件）
go build -trimpath -o onlineTest ./cmd/server

# 2. 启动（默认监听 :8080，数据写入 ./data）
./onlineTest

# 或指定参数
./onlineTest -addr :8080 -data-dir ./data -jwt-days 30 -upload-mb 20

# 3. 浏览器打开
# http://127.0.0.1:8080
```

Windows 下可一键交叉编译 Windows / Linux 两个平台：

```powershell
.\build.ps1
# 仅构建指定平台/架构
.\build.ps1 -Platforms windows -Architectures amd64,arm64
# 构建前先跑测试
.\build.ps1 -Test
```

支持参数：

- `-Platforms windows,linux`：目标平台，默认同时构建 Windows 与 Linux
- `-Architectures amd64,arm64`：目标架构，默认 `amd64`
- `-Test`：构建前运行测试
- `-NoStrip`：不添加 `-ldflags "-s -w"`（保留调试信息）
- `-NoClean`：保留已有的目标目录，不删除旧产物
- `-OutputDir`：自定义输出目录（默认 `dist`）

构建使用 `CGO_ENABLED=0`、`-trimpath`、`-ldflags "-s -w"`，支持纯 Go 跨平台编译。默认输出 `dist/windows-amd64/onlineTest.exe` 与 `dist/linux-amd64/onlineTest`。

首次启动会自动建库、写入初高中科目，并创建老师账号：

- 用户名：`admin`
- 初始密码：未设置 `ONLINETEST_ADMIN_PASSWORD` 时随机生成，只在启动日志打印一次；登录后请立即在「修改密码」中更换。

开发调试可直接用环境变量固定密码（注意 Windows PowerShell 与 CMD 的写法不同）：

```powershell
# PowerShell
$env:ONLINETEST_ADMIN_PASSWORD="admin123"
.\onlineTest.exe
```

```cmd
:: CMD
set ONLINETEST_ADMIN_PASSWORD=admin123
onlineTest.exe
```

```bash
# Git Bash / Linux / macOS
ONLINETEST_ADMIN_PASSWORD=admin123 ./onlineTest.exe
```

Windows 下也可以直接双击 `start.cmd`（会设置初始密码并启动）。若 `data/` 已存在，环境变量不会修改已有账号密码。

## 配置

优先级：命令行参数 > 环境变量 > `config.yaml` > 默认值。

| 参数 | 环境变量 | 默认 | 说明 |
| --- | --- | --- | --- |
| `-addr` | `ONLINETEST_ADDR` | `:8080` | 监听地址 |
| `-data-dir` | `ONLINETEST_DATA_DIR` | `./data` | 数据库、上传文件、密钥目录 |
| `-jwt-days` | `ONLINETEST_JWT_DAYS` | `30` | 登录有效期（天） |
| `-upload-mb` | `ONLINETEST_UPLOAD_MB` | `20` | 单文件上传上限 |
| `-log-level` | `ONLINETEST_LOG_LEVEL` | `info` | debug / info / warn / error |
| `-config` | `ONLINETEST_CONFIG` | `./config.yaml` | 可选 YAML 配置文件（见 `config.example.yaml`，每个字段均有注释） |
| — | `ONLINETEST_SECRET` | 自动生成 | JWT / API Key 加密主密钥；不设置时写入 `data/.secret`（0600） |
| — | `ONLINETEST_ADMIN_PASSWORD` | 随机 | 首次启动创建 admin 时使用 |

数据目录结构：

```
data/
├── app.db        SQLite（WAL）
├── uploads/      试卷图片 / PDF
├── logs/
└── .secret       首次启动生成的主密钥（务必备份）
```

## 配置 AI 模型

进入「AI 模型」→「添加模型」：

| 字段 | 示例 |
| --- | --- |
| Base URL | `https://api.deepseek.com/v1`、`http://127.0.0.1:11434/v1` |
| 模型名 | `deepseek-chat`、`qwen-vl-max`、`llama3.1` |
| API Key | 按服务商填写；加密存储，列表只显示末 4 位 |
| 视觉能力 | 识别图片/扫描试卷时必须开启 |
| 思考模式 | 服务商默认 / 关闭 / 低 / 高 / 最高，映射为 `reasoning_effort` |
| 角色绑定 | 试卷识别（视觉模型）、主观题阅卷、通用/测试、默认兜底 |

保存后点「测试连接」确认可用。未配置模型时，试卷识别任务会给出明确提示；主观题会标记为「待人工复核」，客观题仍正常判分。

## 使用流程

1. 老师「学生管理」创建学生账号（用户名 + 初始密码）。
2. 「科目管理」确认学段/科目（默认已初始化初中、高中各 9 科）。
3. 「试卷管理」→「新建 / 上传试卷」：
   - 先创建空白试卷，再在详情页上传题目截图；或
   - 上传 JPG/PNG/WebP/PDF（可一次选多个，单次最多 30 个），也可以截图后直接 **Ctrl+V** 粘贴到附件区；填写识别提示，点「创建并 AI 识别」，等待任务完成后进入校对页。
   - 识别完成后会**自动为缺少答案的题目调用 AI 补全答案与解析**，进度提示会显示「正在解析缺失答案」；失败或部分失败会在任务提示和试卷页给出警告，可人工补充。
   - 已创建的试卷可在详情页继续「追加」上传新页面：原有题目和文件保留，新题目按顺序追加；也可选择「替换全部」重新识别。已发布试卷仍可追加题目，但只能追加，不能替换全部。
   - 重复上传有判定：同一文件按 SHA256 跳过，重复题目按「题型 + 规范化题干 + 选项」跳过，任务提示会显示跳过数量。
   - 识别成功后原始试卷文件会自动删除（页面也不再展示）；解析失败时保留以便重试，重新上传会覆盖/追加。
4. 校对题干、选项、分值后「发布试卷」；发布只做校验，仍缺答案会提示具体题号。
   - 校对页每题「编辑」左侧有「AI 补答案」：点击后 AI 重新解答并复核当前答案，缺失则补全、错误则改正、正确则保留；请求期间按钮禁用并提示「AI 正在作答」。
   - 校对页顶部「添加题目」直接跳转到试卷详情页，在「批量上传 / 增量识别」区域上传题目截图或文件；手动添加题目入口已移除，已有题目仍可点「编辑」修改。
5. 学生登录 → 选择试卷 → 作答（自动保存、答题卡、公式输入）→ 交卷。
   - 作答页可选择「统一交卷」或「边做边看答案」；后者每题作答后可点击「提交本题看答案」，即时显示对错、得分、正确答案与解题思路，两种模式最后均需交卷。
6. 客观题即时判分，主观题由 AI 阅卷并生成报告；老师可在「成绩与报告」查看、人工改分、重新阅卷。

## 开发与测试

```bash
go test ./...          # 单元 + 集成测试（临时 SQLite，不污染 data/）
go vet ./...
```

前端为原生 ESM，修改 `web/` 后重新 `go build` 即可；无需 npm、无需打包。KaTeX / MathLive 位于 `web/vendor/`，随二进制内嵌。

## 目录

```
cmd/server/         入口
internal/config     配置加载
internal/domain     GORM 模型 + JSON 类型
internal/store      数据访问（Repository）与迁移/seed
internal/security   JWT、bcrypt、AES-GCM
internal/ai         OpenAI 兼容客户端、提示词、JSON 容错
internal/service    解析、判分、阅卷、报告、任务队列
internal/filex      上传校验、PDF 文字层抽取
internal/httpapi    路由、中间件、Handler
web/                原生前端（HTML/CSS/ESM + vendor）
doc/                开发计划、数据库设计、API 设计
```

## 已知限制

- 扫描版 PDF（无文字层）无法在纯 Go 下栅格化：请将页面截图后以图片上传；文本型 PDF 可直接识别。
- 试卷默认不允许学生重做（可在创建时打开 `allow_retry`；老师端支持重新阅卷/人工改分）。
- 公式输入 v1 为「多行文本 + MathLive 浮层插入 + KaTeX 预览」，`contenteditable` 内联公式芯片列为后续增强。
- 主观题评分依赖所配置模型的能力与稳定性；失败会自动降级为人工复核，不阻塞出分。
- SQLite 单写者，适合单机/小规模部署；高并发场景建议换 PostgreSQL 并增加缓存。

## 文档

- [开发计划](doc/开发计划.md)
- [数据库设计](doc/数据库设计.md)
- [API 设计](doc/API设计.md)

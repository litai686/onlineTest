# AGENTS.md

项目内约定与常用命令（在线考试系统：Go + 内嵌 SPA + SQLite）。

## 构建与运行

发布产物只放在 `dist/<os>-<arch>/`：

- 构建全部平台：`.\build.ps1`（默认 windows + linux，`-Test` 构建前跑测试）
- 只构建本机：`.\build.ps1 -Platforms windows`
- 产物路径：`dist/windows-amd64/onlineTest.exe`、`dist/linux-amd64/onlineTest`
- 构建只替换目标目录里的同名二进制，`config.yaml` 等其他文件保持不动（加 `-NoClean` 可连旧二进制也保留）
- 启动开发服务器：`.\start.ps1`（运行 `dist\windows-amd64\onlineTest.exe`，工作目录为仓库根，数据在 `data/`，配置读 `config.yaml`）
- 前端资源经 `web/embed.go` 编译时嵌入二进制，**改完 `web/` 下的文件必须重新执行 `build.ps1` 到 `dist/` 才生效**
- 需要验证时可自行启动服务进程，但**验证完必须立即关闭**（`Stop-Process -Name onlineTest -Force`），不要让它常驻；运行中的 `dist` 二进制会被锁住，导致下次 `build.ps1` 无法覆盖

`go build ./...` 只做编译检查，不要用它产出的二进制作为运行/交付产物。

## 测试

- 全量：`go test ./...`
- 单包：`go test ./internal/httpapi`（接口集成测试）、`go test ./internal/service`

## 配置与数据

- 配置：`config.yaml`（YAML，字段带注释，默认不提交）；优先级 flag > 环境变量 > 文件 > 默认值
- 数据目录：`data/`（`app.db`、`uploads/`、`logs/`、`.secret`），默认不提交
- 首次启动自动建库、写入初高中科目并创建 `admin`

## 代码结构

- `internal/httpapi` 路由与处理函数，错误统一走 `writeError`/`AppError` 返回中文友好文案，禁止把数据库或底层错误原文返回给前端
- `internal/service` 业务逻辑（解析、判分、任务队列、文件与配图清理）
- `internal/store` GORM 数据访问（`migrate.go` 的 `extraDDL` 放索引）
- `internal/domain` 模型与自定义类型；`internal/ai` 提示词与 OpenAI 兼容客户端
- `web/js` 原生 ES Module 前端（`api.js` 统一请求与错误文案，`components/` 复用组件，`pages/` 页面）

## 提交约定

- 提交信息用简体中文，`feat|fix|refactor(scope): 说明` + 圆点列表正文
- 不提交二进制、`dist/`、`data/`、`config.yaml`

## Why

用户需要一个轻量级的 OpenAI API 代理工具，聚合多家主流 AI 厂商接口（OpenAI 兼容、Anthropic、Gemini），统一以 OpenAI 格式对外提供服务。参考 new-api 的功能设计，但针对个人/小团队本机或局域网使用，去除用户体系等重量级功能，以单一 Windows 桌面应用（Wails GUI）形式提供，强调轻量、易用、可单文件分发。

## What Changes

- 新增 Go 1.26 项目，以 Wails GUI 作为桌面管理端，内置本地 HTTP 代理服务
- 支持添加多个厂商渠道（Channel），每个渠道含 Base URL、API Key 列表、优先级、启停状态
- 提供统一 OpenAI 格式代理 API：`/v1/chat/completions`、`/v1/models`、`/v1/embeddings`，支持流式（SSE）透传
- 内置 Anthropic、Gemini 适配层，将非 OpenAI 格式厂商转换为 OpenAI 格式
- 代理端支持 Bearer Token 鉴权（可选关闭，便于本机使用）
- 定时自动刷新渠道模型列表，聚合生成统一模型列表；支持手动刷新
- 故障转移：目标模型在多个渠道可用时，按优先级依次尝试，失败自动切换下一渠道，带熔断冷却机制
- 用量统计：记录每次请求的输入/输出 token、调用次数、耗时等，支持按日与按时段（小时）维度查看
- 使用 SQLite（纯 Go 驱动 modernc.org/sqlite）持久化数据，无 CGO，支持 Windows 单文件交叉编译
- 桌面 GUI 提供仪表盘、渠道管理、模型列表、用量统计、请求日志、设置等页面

## Capabilities

### New Capabilities

- `channel-management`: 渠道的增删改查、启停、优先级排序、连接测试与模型同步触发。渠道按厂商类型分类（OpenAI 兼容 / Anthropic / Gemini / 自定义），每个渠道可配置多个 API Key 轮询。
- `openai-proxy-api`: 统一 OpenAI 格式代理服务，包括鉴权中间件、`/v1/chat/completions`、`/v1/models`、`/v1/embeddings` 路由、流式/普通透传、Anthropic 与 Gemini 格式转换适配。
- `failover-and-balancing`: 请求路由与故障转移。同一模型多渠道可用时按优先级选择，失败自动切换下一渠道；连续失败触发熔断冷却并自动恢复。
- `model-sync`: 模型列表自动刷新。后台定时任务遍历启用渠道拉取模型列表并持久化，聚合生成统一模型列表，失败不中断服务。
- `usage-stats`: 用量统计。记录每次请求的 token 用量与调用信息，支持按日、按时段（小时）聚合查询，供 GUI 图表展示。
- `wails-gui-desktop`: Wails 桌面管理界面（仪表盘、渠道管理、模型列表、用量统计、请求日志、设置），代理服务启停控制，运行时配置管理。与代理服务同进程运行。

### Modified Capabilities

（无，本项目为全新项目，不存在既有规范。）

## Impact

- 全新 Go 项目，目录结构位于仓库根目录：`main.go` 与 `internal/` 包
- 依赖：`github.com/wailsapp/wails/v2`（GUI）、`modernc.org/sqlite`（纯 Go SQLite 驱动）、标准库 `net/http`
- 数据：本地 SQLite 数据库文件（如 `aiproxy.db`），存放渠道、渠道模型映射、用量记录
- 对外接口：本地 HTTP 代理服务（默认端口可配置），监听地址默认 127.0.0.1
- 构建：提供 Windows 交叉编译脚本，产出单文件可执行程序
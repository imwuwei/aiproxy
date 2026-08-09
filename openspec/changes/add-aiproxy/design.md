## Context

本项目为全新项目（仓库当前仅有 openspec 目录），目标是构建一个轻量级 OpenAI API 代理工具，参考 new-api 的功能设计但面向个人/小团队本机或局域网单机使用。已确认的关键约束：

- Go 1.26 + Wails v2 GUI，编译为 Windows 单文件桌面应用
- SQLite 存储（modernc.org/sqlite 纯 Go 驱动，无 CGO，利于跨平台编译）
- 代理端 Bearer Token 鉴权（可选关闭）
- 提供 OpenAI 兼容格式统一入口，支持 OpenAI 兼容 / Anthropic / Gemini 三类厂商

规格文档已定义 6 个能力域：channel-management、openai-proxy-api、failover-and-balancing、model-sync、usage-stats、wails-gui-desktop。

## Goals / Non-Goals

**Goals:**
- 单一可执行文件同时提供 Wails 桌面 GUI 与本地 HTTP 代理服务
- 多渠道聚合管理：按优先级路由、故障转移、熔断冷却
- 模型列表自动同步 + 聚合，`/v1/models` 返回统一列表
- 完整请求用量记录：按日、按时段统计
- Linux 上可交叉编译 Windows 单文件

**Non-Goals:**
- 多用户体系、多令牌管理与独立限额（new-api 的重量级功能，本方案仅单一访问令牌）
- 多机分布式部署、集中式配置中心
- 计费、充值、兑换码等商业化功能
- 请求重放、缓存层（后续可选）
- 原生 GUI 精美主题（保持轻量）

## Decisions

### D1: Wails v2 作为 GUI 框架
- **选择**：Wails v2，代理由标准库 `net/http` 实现，与 GUI 同进程。
- **理由**：Go 后端 + Web 前端，界面表现力强；单文件分发；系统托盘与后台运行支持完善。
- **备选**：本地 Web 服务 + 浏览器（非真正桌面应用，托盘/后台不便）。
- **权衡**：前端为 HTML/JS 资源内嵌，体积略增，但满足管理页需求。

### D2: 纯 Go SQLite（modernc.org/sqlite）
- **选择**：`modernc.org/sqlite`，标准 `database/sql` 接口。
- **理由**：无 CGO，Windows 可交叉编译；单文件部署无 DLL 依赖。
- **备选**：mattn/go-sqlite3（需 CGO，交叉编译麻烦）；bbolt（非 SQL，聚合统计不便）。
- **权衡**：modernc 驱动为 SQL 翻译实现，性能对单机桌面场景完全够用。

### D3: 中继（Relay）接口 + 适配器模式
- **选择**：定义 `Relay` 接口，`OpenAI Compatible` 直接透传；`Anthropic`、`Gemini` 实现格式转换适配器。`relay.go` 提供请求/响应转换入口。
- **理由**：渠道厂商类型通过注册表映射到 Relay 实现，新增厂商（Ollama、Azure 等）只需注册新适配器，符合开闭原则。
- **备选**：按厂商类型硬编码分支（扩展性差）；统一转成第三方 SDK（依赖重）。
- **权衡**：自研转换逻辑需维护 Anthropic/Gemini 的格式差异，但可控。

### D4: 优先级路由 + 熔断故障转移
- **选择**：
  - 路由查询：请求时从 `channel_models` 查含目标模型的启用渠道，按（优先级 ASC, id ASC）排序。
  - 故障转移：非流式请求仅在收到厂商错误响应（401/429/5xx/网络错误）后切换下一渠道；流式请求仅在首包前允许切换。
  - 熔断：内存中维护每渠道连续失败计数与冷却截止时间；连续失败 ≥ 阈值（默认 5）进入冷却（默认 30s）；成功清零。
- **理由**：内存熔断状态轻量，无需持久化；按优先级排序保证确定性。
- **备选**：加权随机负载均衡（更适合高并发多路复用，个人场景优先级确定性更重要）；分布式一致性哈希（超出范围）。
- **权衡**：优先级策略在大并发下不如加权均衡分散流量，但满足"主备切换"的核心诉求。

### D5: 模型同步使用定时任务 + 手动触发
- **选择**：`time.Ticker` 后台任务，默认间隔 1 小时；渠道启用/手动刷新/应用启动时立即执行。刷新结果写 `channel_models`（先删后插，覆盖旧映射）。
- **理由**：模型列表为低频变化数据，定时拉取足够；手动刷新提供即时性。
- **备选**：请求时动态探测（延迟高、不可控）；Webhook 推送（供给侧不支持）。
- **权衡**：定时刷新有最长 1 小时的模型更新延迟，可接受。

### D6: 配置持久化到 SQLite settings 表
- **选择**：单行 key-value `settings` 表存储运行时配置（监听地址、端口、令牌、鉴权开关、刷新间隔、超时、熔断参数）。GUI 修改后立即写库并热生效（代理服务重启监听需要重启服务）。
- **理由**：与业务数据同库，无需额外配置文件；Windows 单文件分发下不产生额外散文件。
- **备选**：独立 JSON/YAML 配置文件（多一个文件，用户易误删）；注册表（跨平台差）。
- **权衡**：首次使用需 GUI 设置，但有合理默认值；数据库文件缺失时自动初始化。

### D7: 数据模型
- **channels**：id, name, type, base_url, api_keys(JSON 数组), priority, enabled, status, last_error, last_success_at, created_at, updated_at
- **channel_models**：id, channel_id, model（联合唯一 (channel_id, model)）
- **usage_records**：id, created_at, channel_id, model, prompt_tokens, completion_tokens, total_tokens, is_success, status_code, duration_ms, error
- **settings**：key, value

### D8: 流式 token 统计
- **选择**：OpenAI 兼容流式 chunk 的 `usage` 字段通常只在末块（`[DONE]` 前）携带；Anthropic/Gemini 转换的流式也尽量在末块附带。若厂商未提供，则用模型上下文窗口估算或记录 0，并在统计页标注估算。
- **理由**：多数厂商最终 chunk 携带 usage，足够精确；避免自行 tokenize（依赖重）。
- **权衡**：个别厂商无 usage 时统计不完整，属可接受范围。

## Risks / Trade-offs

- **前端资源体积** — Wails 内嵌 Web 前端资源，二进制体积比纯原生界面略大 → 前端资源精简，符合轻量定位。
- **Anthropic/Gemini 格式差异** — 参数映射（temperature、top_p、tool 调用等）存在语义差异，转换可能丢失细节 → 首版支持核心字段（messages、model、stream、temperature、max_tokens），其余透传忽略并在文档注明。
- **流式故障转移限制** — 流式响应中途失败无法切换渠道（客户端已收到部分内容） → 在首包前切换；首包后失败直接结束流，符合大多数 OpenAI 客户端行为。
- **SQLite 并发写入** — 高并发下 SQLite 写锁可能成为瓶颈 → 使用 WAL 模式 + 单写连接串行化插入；单机场景并发有限，可接受。
- **modernc 驱动性能** — 相对 CGO 版 sqlite 稍慢 → 数据量小（个人/小团队），影响可忽略。
- **Windows 交叉编译** — Wails 依赖的 WebView2 在 Linux 交叉编译 Windows 时通常可直接构建；若遇到链接问题 → 提供在 Windows 本机构建的脚本作为备选。

## 1. 项目初始化

- [x] 1.1 初始化 go.mod（go 1.26）与目录结构（internal/config、database、models、store、proxy、stats、wailsapp）
- [x] 1.2 添加依赖：github.com/wailsapp/wails/v2、modernc.org/sqlite
- [x] 1.3 实现 internal/models 数据模型（Channel、ChannelModel、UsageRecord、Settings）
- [x] 1.4 实现 database 初始化：SQLite 连接（WAL 模式）、建表迁移、默认设置种子

## 2. 数据访问层（store）

- [x] 2.1 实现 ChannelStore：渠道 CRUD、启停、按优先级查询、状态更新
- [x] 2.2 实现 ChannelModelStore：渠道-模型映射覆盖写入、按模型查渠道、删除渠道映射
- [x] 2.3 实现 UsageStore：用量记录插入、按日聚合查询、按时段聚合查询、按渠道/模型过滤
- [x] 2.4 实现 SettingsStore：配置读写（监听地址、端口、令牌、鉴权开关、刷新间隔、超时、熔断参数）

## 3. 代理核心（proxy）

- [x] 3.1 实现配置热更新机制与鉴权中间件（Bearer Token，可关闭，401 响应）
- [x] 3.2 实现路由注册：POST /v1/chat/completions、GET /v1/models、POST /v1/embeddings
- [x] 3.3 实现 Relay 接口与 OpenAI 兼容直通中继（透传请求/响应，支持流式 SSE）
- [x] 3.4 实现 Anthropic 适配器：OpenAI 请求体 ↔ Anthropic messages 格式双向转换（含流式）
- [x] 3.5 实现 Gemini 适配器：OpenAI 请求体 ↔ Gemini generateContent 格式双向转换（含流式）
- [x] 3.6 实现 Balancer：按（优先级 ASC, id ASC）选择渠道、跳过冷却渠道、故障转移循环重试、流式首包前切换
- [x] 3.7 实现熔断状态管理：连续失败计数、冷却截止时间、成功清零、冷却自动恢复
- [x] 3.8 实现模型路由解析与错误响应：模型不存在 404、全渠道失败聚合错误（OpenAI 格式）

## 4. 模型同步（model-sync）

- [x] 4.1 实现模型拉取器：按渠道类型调用厂商模型列表接口并归一化
- [x] 4.2 实现 ModelSync 服务：单渠道手动刷新、全渠道定时刷新（time.Ticker）、先删后插覆盖映射
- [x] 4.3 实现聚合模型列表查询：启用渠道模型去重集合，供 /v1/models 响应
- [x] 4.4 实现刷新失败状态标记：记录渠道错误、更新渠道状态、不中断服务

## 5. 用量统计（usage-stats）

- [x] 5.1 实现请求用量埋点：非流式从响应 usage 解析、流式从 chunk 累计、失败请求记录
- [x] 5.2 实现按日统计查询（date、count、prompt_tokens、completion_tokens、success、fail）
- [x] 5.3 实现按时段统计查询（0-23 小时：count、prompt_tokens、completion_tokens）

## 6. 桌面 GUI（wails-gui-desktop）

- [x] 6.1 实现主窗口框架：侧边栏导航、应用启动时自动启动代理服务、退出时停止服务
- [x] 6.2 实现仪表盘页：今日调用/Token、渠道状态卡、代理地址复制、启停按钮
- [x] 6.3 实现渠道管理页：表格列表、新增/编辑对话框、删除确认、启停切换、连接测试、手动刷新模型
- [x] 6.4 实现模型列表页：聚合模型 + 渠道数展示
- [x] 6.5 实现用量统计页：日期范围选择（7 天/30 天/自定义）、按日表与按时段表
- [x] 6.6 实现请求日志页：最近请求表格、按渠道/模型筛选下拉
- [x] 6.7 实现设置页：监听地址/端口、令牌、鉴权开关、刷新间隔、超时、熔断参数；保存即生效
- [x] 6.8 实现系统托盘：关闭窗口最小化到托盘、托盘菜单显示/退出、应用退出停止代理

## 7. 收尾与构建

- [x] 7.1 编写 Windows 交叉编译脚本 scripts/build.sh（Wails 桌面版需 CGO/mingw-w64 交叉编译；CLI 版 CGO_ENABLED=0）
- [x] 7.2 编写 README：功能说明、使用指南、厂商配置示例（OpenAI/Anthropic/Gemini）
- [x] 7.3 整体编译验证：go build ./... 与 go vet ./... 通过

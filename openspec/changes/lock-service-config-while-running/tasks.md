## 1. 后端改造（internal/wailsapp/app.go）

- [x] 1.1 `SettingsData` 增加 `ProxyRunning bool` 字段（json: `proxy_running`），`GetSettings()` 从 `a.proxySrv.Running()` 填充
- [x] 1.2 新增私有方法 `serviceConfigChanged(s SettingsData, cfg *config.Config) bool`，比对监听地址/端口/访问令牌/鉴权开关四项是否变化
- [x] 1.3 `SaveSettings()` 在校验阶段增加：服务运行中且服务配置项发生变化时返回错误 `"代理服务运行中，请先停止服务后再修改服务配置"`，不写入数据库

## 2. 前端 - 设置页服务配置卡片锁定与启停按钮（index.html / app.js / style.css）

- [x] 2.1 `index.html` 服务配置卡片新增运行状态行：状态标签 + 同一"启动/停止服务"按钮（`#set-toggle-service`）
- [x] 2.2 `app.js` `loadSettings()` 接收 `proxy_running`，新增 `updateServiceConfigLock(running)`：运行中禁用 `#set-listen-addr`、`#set-listen-port`、`#set-token`、`#set-token-gen`、`#set-auth`；停止状态则解除禁用
- [x] 2.3 `app.js` 设置页启停按钮绑定 `ToggleProxy`，点击后刷新运行状态与锁定态
- [x] 2.4 `state:changed` 事件处理器增加 settings 分支：仅刷新服务运行状态与锁定态，不整体重载设置表单
- [x] 2.5 `style.css` 新增 `input:disabled`/`select:disabled`/`textarea:disabled` 禁用态样式
- [x] 2.6 用户反馈删除锁定提示条：移除 `index.html` 的 `.lock-notice` 提示条与"停止服务"快捷按钮、`app.js` 对应 hide 逻辑与事件绑定、`style.css` 对应样式

## 3. 前端 - 移除仪表盘启停开关（index.html / app.js）

- [x] 3.1 删除 `index.html` 仪表盘 `#dash-toggle` 按钮
- [x] 3.2 删除 `app.js` 中 `dash-toggle` 点击事件绑定；`refreshDashboard` 仅更新状态文本、地址与监听端口，不再操作启停按钮

## 4. 构建验证

- [x] 4.1 `go build` 通过：`internal/wailsapp` 包 Windows 交叉编译（production windows 标签）通过；CLI 构建通过
- [ ] 4.2 交互验证（需 GUI 环境）：服务停止态可编辑/保存服务配置并启动生效；运行态服务配置锁定且保存被拒；非服务配置运行中可保存

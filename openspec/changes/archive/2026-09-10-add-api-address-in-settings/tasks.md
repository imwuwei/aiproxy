## 1. 后端改造（internal/wailsapp/app.go）

- [x] 1.1 `SettingsData` 新增 `BaseURL string` 字段（json: `base_url`），位于 `ListenPort` 之后
- [x] 1.2 `GetSettings()` 填充 `BaseURL: a.config.BaseURL()`，与 `GetDashboard` 同源

## 2. 前端 - 设置页服务配置卡片 API 地址展示与复制（index.html / app.js）

- [x] 2.1 `index.html` 服务配置卡片服务状态行下方新增「API 地址」行：`<code id="set-api-addr" class="mono">` + `<button id="set-api-addr-copy" class="btn">复制</button>`（`.form-row` + `.inline-group` 布局）
- [x] 2.2 `app.js` `loadSettings()` 填充 `byId("set-api-addr").textContent = s.base_url`
- [x] 2.3 `app.js` 新增 `#set-api-addr-copy` 点击绑定：`App.CopyText(#set-api-addr 文本)` + `toast("已复制地址")`
- [x] 2.4 `app.js` `refreshSettingsState()` 同步刷新 `#set-api-addr`（基于 `GetSettings` 返回的 `base_url`）

## 3. 构建验证

- [x] 3.1 后端编译通过：`go build ./...`（或 `go vet ./internal/wailsapp/...`）
- [x] 3.2 前端 JS 语法校验：`node --check app.js`
- [x] 3.3 `openspec-cn validate --changes add-api-address-in-settings` 校验变更制品

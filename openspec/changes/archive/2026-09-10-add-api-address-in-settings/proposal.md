## Why

用户在设置页配置/查看服务时，无法直接看到 API 基础地址（OpenAI 兼容接口地址），必须切换到仪表盘才能查看或复制，造成不必要的往返。设置页服务配置卡片已展示监听地址与端口，但缺少完整可复制的 API 地址。

## What Changes

- 后端 `SettingsData` 新增 `base_url` 字段，`GetSettings()` 返回当前配置计算出的 OpenAI 兼容 API 基础地址（`http://<监听地址>:<端口>/v1`），与仪表盘 `GetDashboard` 的来源保持一致。
- 设置页「服务配置」卡片在服务状态行下方新增「API 地址」展示行（等宽 `code` 样式），并附「复制」按钮，点击一键复制 API 地址。
- 设置页进入时与 `state:changed` 事件时刷新 API 地址展示（服务配置保存后地址即时更新）。
- 无破坏性变更：仅新增展示能力，不改动现有保存/启停流程。

## Capabilities

### New Capabilities

（无新增能力）

### Modified Capabilities

- `wails-gui-desktop`: 「设置页面」需求变更——服务配置卡片新增 API 地址展示与一键复制。

## Impact

- `internal/wailsapp/app.go`：`SettingsData` 新增 `base_url` 字段；`GetSettings()` 填充 `a.config.BaseURL()`。
- `internal/wailsapp/frontend/index.html`：服务配置卡片新增 API 地址展示行与复制按钮。
- `internal/wailsapp/frontend/app.js`：`loadSettings()` 与 `refreshSettingsState()` 填充/刷新 API 地址；新增复制按钮事件绑定（复用 `App.CopyText` + `toast`）。

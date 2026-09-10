## Context

- 动机参见 proposal.md - Why：设置页服务配置卡片缺少 API 地址展示与一键复制。
- 现状：仪表盘已通过 `GetDashboard()` 返回 `base_url`（`a.config.BaseURL()` = `http://<监听地址>:<端口>/v1`）展示并可一键复制；设置页 `GetSettings()` 返回的 `SettingsData` 尚无 `base_url`。
- 复制到剪贴板全站统一走后端绑定 `window.go.wailsapp.App.CopyText(text)` + `toast()`（Wails runtime.ClipboardSetText），不使用 `navigator.clipboard`。
- 设置页状态刷新约束：`state:changed` 事件时仅调用 `refreshSettingsState()`（只刷新运行状态与锁定态），不整体重载表单，避免覆盖用户正在编辑的内容。

## Goals / Non-Goals

**Goals:**
- 设置页「服务配置」卡片展示 OpenAI 兼容 API 基础地址（含 `http://` 与 `/v1`）。
- 提供一键复制按钮，交互与全站一致。
- 服务配置保存/状态变更后地址展示保持最新。

**Non-Goals:**
- 不改变现有服务配置保存、启停、锁定流程。
- 不重构仪表盘已有的地址展示实现。
- 不改动 `config.BaseURL()` 的生成逻辑。

## Decisions

### 决策 1：后端在 `SettingsData` 中返回 `base_url`
- `SettingsData` 新增 `BaseURL string` 字段（json: `base_url`），`GetSettings()` 填充 `a.config.BaseURL()`。
- **理由**：与仪表盘 `GetDashboard` 完全同源，避免前端重复拼接监听地址与端口（`ListenAddr` 可能为 `0.0.0.0` 等），保证两处显示一致。
- **备选**：前端由 `listen_addr` + `listen_port` 自行拼接。放弃原因：复制了后端格式化逻辑，易与 `config.BaseURL()` 漂移。

### 决策 2：UI 采用 `.form-row` + `.inline-group`，置于服务状态行下方
- 新增一行：`<label>API 地址</label>` + `.inline-group` 内为 `<code id="set-api-addr" class="mono">` 与「复制」按钮。
- **理由**：与同卡片「访问令牌」行（input + 复制 + 随机生成）的布局一致；放置于状态行正下方，与仪表盘「状态 → API 地址」层级一致，突出显示。
- **备选**：仿仪表盘 `.service-row` 无按钮布局。放弃原因：设置页表单区统一使用 `.form-row`，复制按钮就近更顺手。

### 决策 3：复制按钮复用 `App.CopyText` + `toast`
- 点击时复制 `#set-api-addr` 的文本并提示「已复制地址」，与 `#dash-copy-addr` 行为一致。

### 决策 4：`loadSettings()` 与 `refreshSettingsState()` 双处刷新地址
- `loadSettings()` 进入设置页时填充 `#set-api-addr`。
- `refreshSettingsState()` 在 `state:changed` 时同步更新 `#set-api-addr`（只读元素，不覆盖用户表单编辑），保证停止服务、保存服务配置等操作后地址即时刷新。

## Risks / Trade-offs

- [设置页与仪表盘地址可能在极短窗口内不一致（二者分别刷新）] → 两者都读取同一 `a.config.BaseURL()`，仅刷新时机不同，实际不会持久不一致。
- [`#set-api-addr` 为只读展示，长地址换行展示可能影响卡片高度] → `.mono` 样式自带内边距，flex 布局下自然换行，无实际风险。
- [`refreshSettingsState()` 每次 `state:changed` 多调用一次 `GetSettings`] → 该接口轻量，且事件并非高频，可接受。

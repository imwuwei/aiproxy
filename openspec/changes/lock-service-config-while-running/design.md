## Context

当前设置页"服务配置"卡片允许在代理服务运行中修改监听地址、监听端口、访问令牌与鉴权开关。技术现状：

- `proxy.Server.Start()` 启动时以当时的配置构造 `http.Server.Addr`，运行中修改监听地址/端口不生效。
- `SaveSettings` 保存后调用 `proxySrv.ReloadConfig()`，仅热更新 `http.Client` 超时、熔断参数与鉴权快照，不重启监听。
- 后端 `Server` 提供 `Running()`、`Start()`、`Stop()`、`ToggleProxy()`（在 `internal/wailsapp/app.go`）。
- 前端设置页 `loadSettings()` 每次进入 / `state:changed` 时刷新，仪表盘 `dash-toggle` 提供启停开关。

因此服务配置（尤其监听地址/端口）在运行中修改会造成"保存成功但实监听不变"的误导，需要改为"先停止服务才能修改"。

## Goals / Non-Goals

**Goals:**
- 服务运行中禁止修改服务配置 4 项（监听地址、监听端口、访问令牌、鉴权开关），前端锁定 + 后端兜底校验。
- 设置页服务配置卡片提供同一"启动/停止服务"按钮，替代仪表盘启停开关。
- 非服务配置项（模型同步间隔、超时、熔断、日志保留、调试）运行中仍可保存并立即生效。

**Non-Goals:**
- 不实现运行时自动重启监听（不修改 `proxy.Server` 的监听重建能力）。
- 不改动 `proxy.Server` 的 `Start/Stop/Running` 接口语义（仍为单次启停，非幂等切换语义）。
- 不调整 CLI（`main_cli.go`）行为，仅作用于 Wails GUI。

## Decisions

### 决策 1：前端锁定服务配置表单项（主防线）
- 在 `app.go` 的 `SettingsData` 增加 `ProxyRunning bool` 字段，`GetSettings()` 从 `a.proxySrv.Running()` 填充。
- 前端 `loadSettings()` 收到 `proxy_running` 后调用 `updateServiceConfigLock(running)`：
  - 运行中：禁用 `#set-listen-addr`、`#set-listen-port`、`#set-token`、`#set-token-gen`、`#set-auth`。
  - 已停止：解除禁用；`#set-token-copy`（只读复制）始终可用，不参与禁用。
- 引导方式：不新增提示条。服务状态行与同一"启动/停止服务"按钮（`#set-toggle-service`）即提供停止入口，用户停止后表单项自动解锁。
- **备选方案**：仅靠后端校验、前端不做禁用。放弃原因：用户体验差，用户输入半天保存才报错；锁定表单可即时引导。

### 决策 2：后端 SaveSettings 兜底校验（后防线）
- 在 `SaveSettings` 中，先比较提交的 `s` 与当前 `a.config` 的服务配置 4 项是否变化，若 `a.proxySrv.Running()` 且存在变化，直接返回错误，不写数据库。
- 新增私有方法 `serviceConfigChanged(s SettingsData) bool`（内部读取 `a.config` 比对）：
  ```
  !strings.EqualFold 比较 ListenAddr；
  ListenPort 不等；
  AccessToken 不等；
  AuthEnabled 不等。
  ```
- **备选方案**：不做后端校验。放弃原因：Wails 绑定方法可能被原生调用方/自动化直接调用，需防止绕过前端。

### 决策 3：启停控制迁移到设置页
- `index.html` 服务配置卡片顶部新增状态行：运行状态标签 + 同一"启动/停止服务"按钮（复用 `App.ToggleProxy`，ID 为 `#set-toggle-service`）。
- 删除仪表盘 `#dash-toggle` 按钮及 `app.js` 对应点击绑定；仪表盘保留状态点、地址展示与复制按钮，`refreshDashboard` 仅更新状态文本不再改按钮。
- 设置页启停按钮点击后：调用 `ToggleProxy()` → 刷新运行状态与锁定态（`loadSettings`）→ 由于 `ToggleProxy` 已 `emitStateChanged`，`state:changed` 处理器会刷新设置页锁定态。
- **备选方案**：保留仪表盘开关 + 设置页新增开关（两处入口）。放弃原因：用户明确要求删除仪表盘开关，单一入口更清晰，且避免状态同步复杂度。

### 决策 4：`state:changed` 事件下设置页只刷新锁定态
- 现有 `state:changed` 处理器对设置页无处理（`refreshers` 不含 settings）。本次显式为 settings 分支：仅调用 `refreshSettingsState` / `updateServiceConfigLock` 刷新，**不**整体 `loadSettings`，避免覆盖用户编辑中其他卡片的内容。
- 保存行为不变：`settings-save` 仍提交整个表单；服务停止后用户可修改服务配置项并保存。

## Risks / Trade-offs

- [后端校验依赖 `a.config` 与提交值逐项比对，鉴权开关与令牌组合可能出现"副本不一致"误判] → 校验仅针对 4 项精确字段比对，且 `GetSettings` 与校验都基于同一 `a.config` 来源；命中后返回具体错误文案引导先停止服务。
- [用户在其他卡片编辑了非服务配置，但服务配置项有历史未保存改动被误判] → 前端锁定确保运行中服务配置项不会产生改动；后端比对的是提交值 vs 当前配置，仅在实际变化时拦截，不会误拦未修改情况。
- [删除仪表盘开关后，新用户可能找不到启停入口] → 设置页服务配置卡片增加醒目状态行与"启动/停止服务"按钮，作为唯一启停入口。
- [`ToggleProxy` 在设置页调用时，仪表盘不再显示启停] → 仪表盘状态文本随 `state:changed` 自动刷新，保持一致。

## Migration Plan

1. 后端先行：`SettingsData` 加字段、`SaveSettings` 加校验（保证即使前端未改也不会出现运行中改服务配置的路径）。
2. 前端改造：设置页锁定与启停按钮；删除仪表盘开关。
3. 验证：停止态可编辑/保存并启动生效；运行态锁定与保存拒绝；非服务配置运行中可保存。
4. 回滚：恢复 `dash-toggle` 按钮与 `SaveSettings` 校验分支即可，无数据迁移。

## Open Questions

- 无。
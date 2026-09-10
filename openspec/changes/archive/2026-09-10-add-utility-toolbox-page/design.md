## Context

参见 proposal.md - Why。技术现状：

- 前端为原生 JS 单页应用（index.html / app.js / style.css，无框架），页面切换由 `showPage(name)` 驱动：隐藏全部 `.page`、显示 `#page-<name>`，并按 `refreshers` 映射在进入页面时刷新数据；导航按钮带 `data-page`。
- 现有页面：dashboard、channels、aliases、models、stats、logs、settings。
- 复制到剪贴板统一走后端绑定 `window.go.wailsapp.App.CopyText(text)` + `toast()`（非 navigator.clipboard）。
- 统计页已用 `.tabs`/`.tab` 实现页签，但其 JS 处理器（app.js 约 1394 行）使用**全局选择器** `document.querySelectorAll(".tab")` 维护 `statsTab` 变量——新增同类 class 会被其误捕获并污染统计页状态。

## Goals / Non-Goals

**Goals:**
- 新增第八个页面「小工具」，与现有页面交互与视觉风格一致。
- 五个工具全部纯前端实现，正确处理 Unicode/UTF-8 与密码学安全随机数。
- 页面与工具结构可扩展，便于后续追加新工具。

**Non-Goals:**
- 不新增任何后端 Go 绑定/接口（五个工具不依赖后端能力）。
- 不做数据持久化、不引入使用历史记录。
- 不实现密码强度策略之外的复杂逻辑（仅按选项生成）。
- 不改动既有页面行为（含统计页页签逻辑）。

## Decisions

### 决策 1：新增独立页面，复用现有导航/页面机制

- index.html 导航新增「小工具」导航项，**插入在「设置」之前，设置保持导航最后一位**（导航顺序：仪表盘 → 渠道管理 → 模型别名 → 模型管理 → 用量统计 → 请求日志 → 小工具 → 设置）：`<button class="nav-item" data-page="tools" title="小工具"><span class="nav-icon">🧰</span>小工具</button>`；新增 `<section id="page-tools" class="page" hidden>`。
- `showPage` 的 `refreshers` 不注册 `tools`：纯前端工具进入页面无需刷新（`if (refreshers[name])` 已有空值保护）。
- **备选**：作为设置页内嵌块。放弃：小工具定位独立入口更清晰，且避免设置页膨胀。

### 决策 2：工具选项卡使用独立 class（`.tool-tabs` / `.tool-tab`），避免与统计页 `.tab` 冲突

- 统计页 `.tab` 处理器用全局 `document.querySelectorAll(".tab")` 维护 `statsTab`；若新工具页复用 `.tab` class，切换工具会误改 `statsTab`。
- 方案：工具页使用 `.tool-tab` class + `#page-tools` 作用域的点击处理器；样式通过选择器合并（`.tabs, .tool-tabs { ... }`、`.tab, .tool-tab { ... }`）复用现有页签视觉。
- **备选**：把统计页处理器改为 `#page-stats .tab`。放弃：触碰既有工作代码，回归风险更高。

### 决策 3：各工具实现要点

- **时间戳**：`Date`/`Date.parse`/`getTime`；输入 Unix 秒或毫秒自动判定（数值 `<1e12` 视为秒）；支持「当前时间戳」快捷填充；双向转换并同时展示本地与 UTC。
- **网址**：`encodeURIComponent`/`decodeURIComponent` 做字段级编解码（避免整串编码时 `/` `:` 等被转义的困惑）；`URIError` 捕获并提示。
- **密码**：基于 `crypto.getRandomValues`（密码学安全）；选项含长度（默认 16）、大小写字母/数字/符号开关、排除易混淆字符（如 `0O1lI|`）；保证每种启用的字符集至少出现一次；全部字符集关闭时回退到「大小写+数字」并提示。
- **Base64**：编码 = `TextEncoder` 转 UTF-8 字节 → `btoa`；解码 = `atob` → `TextDecoder`（fatal: false）；非法 Base64 捕获异常并提示，避免中文乱码。
- **正则**：`new RegExp(pattern, flags)`，标志 `g/i/m/s/u` 复选框；`matchAll` 展示每条匹配内容与 index；无效正则捕获异常提示；替换预览用 `String.prototype.replace`。

### 决策 4：结果输出与交互风格

- 每个工具一张 `.card`，内含表单控件与只读结果区；结果区带「复制」按钮（`window.go.wailsapp.App.CopyText` + `toast("已复制")`，与现有页面一致）。
- 输入即算（事件驱动）为主、显式按钮为辅（密码生成、正则匹配/替换需按钮触发）。

## Risks / Trade-offs

- [Base64 用 `btoa`/`atob` 直接处理中文会乱码] → 统一先经 `TextEncoder`/`TextDecoder` 转字节；解码前校验并捕获异常。
- [时间戳 10 位秒 / 13 位毫秒判定可能误判] → 以数量级启发式判定（`<1e12` 按秒），结果区同时展示两种解读供核对。
- [全局 `.tab` 处理器误捕获新页签、污染 `statsTab`] → 使用 `.tool-tab` 独立 class + `#page-tools` 作用域处理器，不触碰统计页代码（决策 2）。
- [密码生成字符集选项全关] → 回退「大小写+数字」并提示，保证始终可生成。
- [新增纯前端代码增大 app.js 体积] → 各工具为独立函数块，按页注释分区，保持现有组织惯例。

## Migration Plan

1. index.html：导航项 + `#page-tools` 空壳结构与五工具面板。
2. style.css：`.tool-tabs`/`.tool-tab` 与结果输出框样式。
3. app.js：工具页 tab 切换、五个工具逻辑、复制绑定。
4. 验证：`make build` 确认前端资源打包；GUI 环境逐一验证五工具（含中文 Base64 往返、秒/毫秒时间戳、正则 flags、密码复制）。
5. 回滚：移除导航项、`#page-tools` 区块及对应 JS/CSS 即可，无数据迁移。

## Open Questions

- 无。

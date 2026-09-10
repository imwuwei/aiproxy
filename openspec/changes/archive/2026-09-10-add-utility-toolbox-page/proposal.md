## Why

开发者在日常使用 AIProxy 桌面应用时，常需要临时进行时间戳转换、URL 编解码、Base64 转换、随机密码生成、正则调试等小工具操作。目前这类操作需要切换到外部工具或浏览器完成，体验割裂。在应用内提供一个轻量、随开随用的「小工具」页面，可显著提升日常调试与开发效率。

## What Changes

- 在 Wails 桌面 GUI 侧边导航中新增「小工具」页面，位于「设置」之前，设置保持导航最后一位。
- 页面以选项卡（tab）形式提供五个纯前端工具：
  - **时间戳转换**：Unix 秒/毫秒时间戳 ↔ 本地时间与 UTC 时间互转，支持当前时间戳快捷填充。
  - **网址转换**：URL 编码/解码（基于 `encodeURIComponent` / `decodeURIComponent`），支持整串与单字段转换。
  - **随机密码生成**：可配置密码长度与字符集（大小写字母、数字、符号、排除易混淆字符），基于 `crypto.getRandomValues` 生成，支持一键复制。
  - **Base64 编解码**：文本 ↔ Base64 互转，使用 `TextEncoder`/`TextDecoder` 正确处理 UTF-8，避免中文等非 ASCII 字符乱码。
  - **正则匹配**：输入正则与待匹配文本，显示匹配结果列表与匹配数量，支持 `g/i/m/s/u` 标志切换，并支持替换预览。
- 全部工具均为纯前端计算，不新增后端接口、不访问网络、不持久化任何数据。
- 页面结构设计为可扩展：每个工具为独立卡片/选项卡，后续可方便追加新工具。

## Capabilities

### New Capabilities

（无新增能力）

### Modified Capabilities

- `wails-gui-desktop`: 新增「小工具」页面需求。GUI SHALL 提供小工具页面，包含时间戳转换、网址转换、随机密码生成、Base64 编解码、正则匹配五个工具，全部为纯前端实现。

## Impact

- `internal/wailsapp/frontend/index.html`：侧边导航新增「小工具」入口按钮；新增 `#page-tools` 页面区块及各工具的表单/结果结构。
- `internal/wailsapp/frontend/app.js`：新增页面切换注册、五个工具的计算/生成/匹配交互逻辑与结果渲染。
- `internal/wailsapp/frontend/style.css`：新增工具卡片、选项卡、结果输出框等样式。
- 无后端 Go 代码改动；无依赖变更；无数据模型变化；不涉及 CLI 与代理服务。

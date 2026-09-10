## 1. 页面结构（index.html）

- [x] 1.1 侧边导航新增「小工具」导航项（插入在「设置」导航项之前，设置保持最后）：`<button class="nav-item" data-page="tools" title="小工具"><span class="nav-icon">🧰</span>小工具</button>`
- [x] 1.2 新增 `<section id="page-tools" class="page" hidden>`：页头（标题+副标题）、`.tool-tabs` 容器（时间戳/网址/密码/Base64/正则五个 `.tool-tab` 按钮）与五个工具面板
- [x] 1.3 时间戳工具面板：时间戳输入、日期时间输入、当前时间戳填充按钮、本地/UTC 结果区与复制按钮
- [x] 1.4 网址工具面板：文本输入、编码/解码操作、结果区与复制按钮
- [x] 1.5 密码工具面板：长度输入、字符集复选框（大小写/数字/符号）、排除易混淆字符开关、生成按钮、结果区与复制按钮
- [x] 1.6 Base64 工具面板：文本输入、Base64 输入、编码/解码操作、结果区与复制按钮
- [x] 1.7 正则工具面板：正则输入、标志复选框（g/i/m/s/u）、替换文本输入、匹配/替换按钮、结果区（匹配列表/数量/替换结果）

## 2. 样式（style.css）

- [x] 2.1 新增 `.tool-tabs`/`.tool-tab` 样式（选择器合并复用 `.tabs`/`.tab` 视觉）
- [x] 2.2 新增工具卡片布局与结果输出框（只读、等宽字体）样式

## 3. 交互逻辑（app.js）

- [x] 3.1 工具页选项卡切换：`#page-tools .tool-tab` 点击切换对应面板（作用域限定，勿用全局 `.tab` 选择器）
- [x] 3.2 时间戳转换逻辑：秒/毫秒自动判定、日期↔时间戳双向转换、本地与 UTC 展示、当前时间戳填充
- [x] 3.3 网址编码/解码逻辑：`encodeURIComponent`/`decodeURIComponent`，异常捕获与提示
- [x] 3.4 随机密码生成逻辑：`crypto.getRandomValues`、字符集选项、每种启用字符集至少出现一次、易混淆字符排除、全关回退
- [x] 3.5 Base64 编解码逻辑：`TextEncoder`/`TextDecoder` + `btoa`/`atob`，UTF-8 正确性、非法输入提示
- [x] 3.6 正则匹配/替换逻辑：`new RegExp`、`matchAll` 匹配列表与位置、无效正则提示、替换预览
- [x] 3.7 各结果区「复制」按钮绑定 `window.go.wailsapp.App.CopyText` + `toast`

## 4. 构建验证

- [x] 4.1 `make build` / 构建脚本通过，前端资源正常打包
- [ ] 4.2 交互验证（需 GUI 环境）：导航进入小工具页；五工具逐一验证（含中文 Base64 往返、秒/毫秒时间戳、正则 flags、密码复制）

# AIProxy - 轻量级 OpenAI API 代理

聚合多家主流 AI 厂商接口（OpenAI 兼容 / Anthropic / Gemini），以统一 OpenAI 格式对外提供代理服务，内置 Wails 桌面管理界面（Windows 优先）。

## 功能特性

- **多渠道聚合**：添加多个厂商渠道，支持配置 API Key 列表、优先级（数值越小越优先）、启停控制
- **统一代理 API**：`POST /v1/chat/completions`、`POST /v1/embeddings`、`GET /v1/models`，支持流式（SSE）透传
- **厂商格式适配**：内置 Anthropic、Gemini 适配器，自动进行 OpenAI ↔ 厂商格式转换
- **模型自动同步**：定时刷新各渠道模型列表并聚合（默认每小时，可配置），支持手动刷新
- **故障转移**：同一模型多渠道可用时按优先级依次尝试；连续失败触发熔断冷却（默认 5 次 / 30 秒）自动恢复
- **模型别名**：为多个模型定义别名，请求别名时按权重轮询并在目标模型之间故障转移，响应中的 model 字段自动还原为别名
- **自定义模型**：可手工添加模型并绑定渠道，与自动同步的模型一样参与路由
- **用量统计**：记录每次请求的输入/输出 token、调用次数、耗时，支持按日与按时段（0-23 小时）查看
- **桌面 GUI**：仪表盘、渠道管理、模型别名、模型管理、用量统计、请求日志、设置七大页面，支持系统托盘
- **安装程序**：提供 Windows 安装包（NSIS），支持自定义安装路径、开机自启动、桌面/开始菜单快捷方式、完整卸载器
- **开机自启动**：安装时勾选或在应用「设置」页随时开关，写入当前用户注册表，登录系统自动运行
- **鉴权控制**：代理端 Bearer Token 鉴权（可关闭，便于本机使用）

## 快速开始

### Windows

#### 方式一：安装程序（推荐）

1. 下载最新版安装包 `aiproxy-Setup-<版本号>.exe`
2. 双击运行安装向导，按提示完成安装：
   - **选择安装路径**：默认 `%LOCALAPPDATA%\Programs\AIProxy`，可点击「浏览」自定义，用户级安装无需管理员权限
   - **选择组件**：勾选「开机自启动」「创建桌面快捷方式」（均默认勾选）
3. 安装完成页可选择「立即运行 AIProxy」启动应用，代理服务自动启动（默认 `http://127.0.0.1:17880`）
4. 在「渠道管理」页新增厂商渠道，填写 Base URL 与 API Key
5. 在「模型管理」页确认模型已同步，即可使用代理

> **卸载**：开始菜单 →「AIProxy」→「卸载 AIProxy」；卸载时询问是否保留 `aiproxy.db` 用户数据（渠道配置、用量统计等），选择「否」将连同数据一并删除。
>
> **重新安装或升级**：安装器检测到已安装版本时，默认沿用原安装路径，程序数据（`aiproxy.db`）不受影响。

#### 方式二：绿色版

1. 下载最新版 `aiproxy-windows-amd64.exe` 直接运行
2. 应用启动后自动启动代理服务（默认 `http://127.0.0.1:17880`）
3. 在「渠道管理」页新增厂商渠道，填写 Base URL 与 API Key
4. 在「模型管理」页确认模型已同步，即可使用代理

### 开机自启动

两种方式启用，效果等价（均写入当前用户的 Windows 注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`，无需管理员权限）：

1. **安装时**：安装向导「选择组件」页勾选「开机自启动」
2. **运行中**：应用「设置」页 →「开机自启动」卡片勾选开关，**切换立即生效**，无需重启

> 注：开机自启动仅 Windows 支持，Linux 等其他平台「设置」页对应开关自动禁用。
>
> 取消自启动：在「设置」页取消勾选，或卸载安装器时会自动清理注册表项。

### 源码构建

```bash
# Windows 安装程序（推荐，需 mingw-w64 + go-winres + nsis）
#   apt install gcc-mingw-w64-x86-64 nsis
#   go install github.com/tc-hib/go-winres@latest
./scripts/build.sh installer    # 输出 build/aiproxy-Setup-<版本号>.exe

# 或指定发布版本号（Makefile 方式，自动规范化为 x.y.z.w 文件版本）
make installer VERSION=1.0.0

# Windows 交叉编译（仅 exe，需 mingw-w64 与 go-winres）
./scripts/build.sh windows

# 本地 Linux 桌面版（需 webkit2gtk-4.0 & gtk+-3.0）
./scripts/build.sh linux

# 或直接（桌面版基于 Wails，必须携带 production tag，否则运行时报
# "Wails applications will not build without the correct build tags"）
go build -tags "production x11" -o build/aiproxy-linux .
```

**Makefile 常用目标**（`make help` 查看全部）：

| 目标 | 说明 |
|---|---|
| `make build` | 构建当前平台桌面版（Wails） |
| `make build-linux` | 构建 Linux 桌面版（需 webkit2gtk-4.0 & gtk+-3.0） |
| `make build-windows` | 交叉编译 Windows 桌面版（需 mingw-w64 + go-winres） |
| `make build-cli` | 构建当前平台命令行版 |
| `make build-cli-windows` | 交叉编译 Windows 命令行版（纯 Go，无需 mingw-w64） |
| `make build-all` | 构建全部平台版本（Linux + Windows 桌面版 + CLI） |
| `make installer` / `make build-installer` | 生成 Windows 安装包（依赖 NSIS） |
| `make run` | 本地运行桌面版 |
| `make test` / `make fmt` / `make vet` / `make tidy` | 测试 / 格式化 / 静态检查 / 整理依赖 |
| `make clean` | 清理构建产物 |

> 安装包基于 [NSIS](https://nsis.sourceforge.io/)（Nullsoft Scriptable Install System）生成，
> 脚本位于 `packaging/nsis/installer.nsi`，可通过 `makensis -DVERSION=x.y.z installer.nsi` 单独编译。

## 命令行版（CLI）

AIProxy 提供完全无 GUI 依赖的纯命令行版本，通过子命令管理渠道、模型、统计、日志与配置，适合服务器部署与脚本化运维。**不依赖任何 GUI 图形库，编译产物无需 CGO/桌面环境依赖。**

### 构建

```bash
# 方式一：构建脚本
./scripts/build.sh cli            # 输出 build/aiproxy-cli（当前平台）
./scripts/build.sh windows-cli    # 交叉编译 Windows 版，输出 build/aiproxy-cli-windows-amd64.exe
./scripts/build.sh all            # GUI（Linux+Windows）+ CLI（Linux+Windows）全部

# 方式二：Makefile
make build-cli                    # 输出 build/aiproxy-cli-<平台>-<架构>
make build-cli-windows            # 交叉编译 Windows 版，输出 build/aiproxy-cli-windows-amd64.exe
```

> **Windows 命令行版**：纯 Go 实现、无 CGO/无 GUI 图形库依赖，可直接在 Windows 的 CMD / PowerShell 中以控制台程序运行（`aiproxy serve` 前台运行，Ctrl+C 停止；管理命令同理）。无需安装 mingw-w64 与 go-winres。
>
> **Windows 交互式提示**：Windows 控制台默认代码页可能影响中文字符显示，若出现乱码可在 CMD 中先执行 `chcp 65001` 切换到 UTF-8 代码页。

### 快速开始

```bash
# 前台启动代理服务（Ctrl+C 停止；含定时模型同步与日志自动清理）
aiproxy serve

# 查看全部命令帮助
aiproxy help

# 查看渠道/模型等子命令帮助
aiproxy channels --help

# 查看版本信息
aiproxy version
```

### 渠道管理

```bash
aiproxy channels list                              # 列出全部渠道
aiproxy channels create \
  --name "OpenAI" \
  --type openai-compatible \
  --base-url "https://api.openai.com" \
  --api-key "sk-xxx" \
  --priority 0                                     # 新增渠道（自动触发模型同步）
aiproxy channels update --id 1 --name "新名称"      # 修改渠道（只修改传入的字段）
aiproxy channels enable --id 1                    # 启用渠道（自动触发模型同步）
aiproxy channels disable --id 1                   # 停用渠道
aiproxy channels test --id 1                      # 测试渠道连通性（返回模型数）
aiproxy channels sync --id 1                      # 同步单个渠道模型
aiproxy channels delete --id 1                    # 删除渠道
```

### 模型管理

```bash
aiproxy models list                       # 列出全部可用模型（含渠道数与 Token 用量）
aiproxy models list-custom                # 列出自定义模型
aiproxy models sync-all                   # 全量同步所有启用渠道的模型
aiproxy models add "my-model" \
  --desc "自定义模型" \
  --channels 1,2                          # 添加自定义模型并绑定渠道
aiproxy models remove "my-model"          # 删除自定义模型
aiproxy models edit "my-model" --desc "新描述"   # 编辑自定义模型描述
aiproxy models bindings "my-model"        # 查看模型绑定的渠道
aiproxy models bind "my-model" --channels 1,3   # 设置模型绑定的渠道（全量覆盖）
```

### 模型别名管理

```bash
aiproxy aliases list                                    # 列出全部模型别名
aiproxy aliases create --name "all" \
  --model gpt-4o --model claude-3-5-sonnet              # 新增别名（权重均等）
aiproxy aliases create --name "mix" \
  --targets '[{"model":"gpt-4o","weight":2},{"model":"gemini-1.5-pro"}]'   # 新增别名（按权重轮询）
aiproxy aliases update --id 1 --enabled false           # 修改别名（只修改传入的字段）
aiproxy aliases enable --id 1                           # 启用别名
aiproxy aliases disable --id 1                          # 停用别名
aiproxy aliases delete --id 1                           # 删除别名
```

> 模型别名：客户端以别名作为 model 请求，代理按权重在多个目标模型间轮询，并支持模型级故障转移；响应中的 model 字段会自动还原为别名。别名与现有模型重名时保留真实模型。

### 用量统计

```bash
aiproxy stats summary [--range today|7d|30d] [--channel <ID>] [--model <模型>]   # 整体汇总（默认今日）
aiproxy stats daily    [--range 7d|30d] [--channel <ID>] [--model <模型>]        # 按日统计
aiproxy stats models   [--range 7d|30d] [--channel <ID>]                         # 按模型统计
aiproxy stats channels [--range 7d|30d] [--model <模型>]                         # 按渠道统计
```

### 请求日志

```bash
aiproxy logs list [--limit 200] [--channel <ID>] [--model <模型>]   # 查看最近请求日志
aiproxy logs clear                                                  # 清空全部请求日志
```

### 配置管理

```bash
aiproxy settings show                                          # 显示当前配置（令牌脱敏）
aiproxy settings set --listen-port 8080 --debug true           # 修改配置（即时/重启生效项自动提示）
aiproxy settings set --access-token sk-新令牌 --auth-enabled true
aiproxy settings gen-token [--save]                            # 生成随机访问令牌
```

### 全局参数

```bash
--db <path>   指定数据库路径（默认: AIPROXY_DB 环境变量，否则可执行文件同目录 aiproxy.db）
--json        以 JSON 格式输出列表/统计/明细（便于脚本解析）
```

> 提示：所有管理命令与 GUI 版共用同一套数据库与配置，可同时使用、互不影响。
> 监听地址/端口修改需重启服务生效，其余配置写入后由服务热加载（服务模式下即时生效）。

### 程序图标

应用内置深蓝渐变 + 白色转发箭头 + 橙色点缀的程序图标：

- **GUI 窗口 / 任务栏 / 系统托盘**：系统托盘图标采用仓库根目录 `assets/aiproxy.png`（构建时由 `main_wails.go` 嵌入；Windows 下运行时转换为 .ico 后加载，其他平台直接供 systray 使用）
- **Windows 可执行文件图标**：构建时由 go-winres 从 `assets/aiproxy.ico` 嵌入（含多尺寸 16/32/48/128/256）
- **Linux 桌面图标**：`assets/aiproxy.png` 可作为 `.desktop` 入口的图标使用

如需重新生成图标资源，依赖 Pillow：

```bash
pip install pillow
python3 scripts/gen_icon.py   # 重新生成 assets/aiproxy.png 与 assets/aiproxy.ico
```

## 使用代理

所有 OpenAI 客户端将 Base URL 指向代理地址即可：

```bash
# Python openai 客户端示例
import openai
client = openai.OpenAI(
    base_url="http://127.0.0.1:17880/v1",
    api_key="sk-aiproxy",   # 默认访问令牌，可在设置页修改
)
resp = client.chat.completions.create(
    model="gpt-4o",  # 使用你在渠道中配置的模型 ID
    messages=[{"role": "user", "content": "你好"}],
)
print(resp.choices[0].message.content)
```

### 目前支持的模型类型

| 渠道类型 | Base URL 示例 | 说明 |
|---|---|---|
| openai-compatible | `https://api.openai.com` | OpenAI 及一切兼容 OpenAI 格式的服务（Ollama、LM Studio、DeepSeek 等） |
| anthropic | `https://api.anthropic.com` | Claude 系列，自动转换消息格式 |
| gemini | `https://generativelanguage.googleapis.com` | Google Gemini，自动转换格式 |
| custom | 自定义 | 与 openai-compatible 相同行为 |

> 提示：API Key 支持填写多个，每行一个，请求时自动轮询。
> 模型同步失败不影响代理服务，失败渠道会标记状态并在下次成功刷新时恢复。

## 数据存储

- 所有数据（渠道、模型映射、用量记录、设置）存储在可执行文件同目录的 `aiproxy.db`（SQLite）
- 可用环境变量 `AIPROXY_DB` 指定数据库路径
- 安装程序卸载时默认提示询问是否保留 `aiproxy.db`，选择保留则重新安装后数据完整可用
- 绿色版（免安装 exe）直接运行同目录生成 `aiproxy.db`，删除整个目录即完成卸载

## 兼容性说明

- Wails 桌面版依赖 CGO（Windows 下 WebView2 后端，Linux 下 webkit2gtk-4.0 & gtk+-3.0），Windows 交叉编译需 mingw-w64 工具链
- Windows 安装包构建额外依赖：NSIS 3.0+（`apt install nsis`）与 go-winres（`go install github.com/tc-hib/go-winres@latest`）
- 非流式响应的故障转移完整重试；流式响应仅在首包前切换渠道，首包后中断则直接结束流

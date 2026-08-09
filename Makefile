# AIProxy Makefile
# 用法: make [build|build-linux|build-windows|build-all|installer|build-installer|run|test|fmt|vet|clean]

# 项目名称
APP_NAME     := aiproxy
BUILD_DIR    := build

# 版本信息（可通过 make VERSION=x.y.z 覆盖）
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME   ?= $(shell date -u '+%Y-%m-%d %H:%M:%S')
GIT_COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# 编译参数
LDFLAGS      := -s -w
# 桌面版（Wails）构建标签：production 为 Wails 框架必需（缺失时运行时报错），
# 其余平台标签按目标系统附加（Linux 默认启用 x11，规避 Wayland 头文件兼容问题）。
DESKTOP_TAGS := production
# CLI 版构建标签
CLI_TAGS     := cli
# Linux 构建标签：默认启用 x11，规避 Wayland 头文件兼容问题（可通过 make TAGS= 覆盖）
LINUX_TAGS   ?= x11
# 如需注入版本信息，可取消注释以下行并添加 Version 变量
# LDFLAGS      += -X "main.version=$(VERSION)" -X "main.buildTime=$(BUILD_TIME)" -X "main.gitCommit=$(GIT_COMMIT)"

# 工具检查
GO           ?= go
MINGW_CC     := x86_64-w64-mingw32-gcc
# go-winres 优先使用 PATH 中的可执行文件，否则回退到 GOPATH/bin
WINRES       ?= $(shell command -v go-winres 2>/dev/null || echo "$(shell $(GO) env GOPATH)/bin/go-winres")
UNAME_S      := $(shell uname -s)
# NSIS 编译器（Linux 下可通过 sudo apt install nsis 安装）
MAKENSIS     ?= $(shell command -v makensis 2>/dev/null || echo "makensis")
# Windows 安装包相关
INSTALLER_NAME := $(APP_NAME)-Setup-$(VERSION).exe
NSIS_SCRIPT  := packaging/nsis/installer.nsi
WIN_EXE      := $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe

# Windows 资源文件（图标 + 版本信息 + 清单），由 go-winres 从 winres/winres.json 生成
WINRES_SYSO  := rsrc_windows_amd64.syso

# 默认目标: 构建当前平台可执行文件（Wails 桌面版）
.PHONY: help
help: ## 显示帮助信息
	@echo "AIProxy Makefile"
	@echo ""
	@echo "Usage:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## 构建当前平台可执行文件（Wails 桌面版）
	@echo ">>> 构建 $(APP_NAME) 当前平台桌面版（Wails）..."
	$(GO) build -tags '$(DESKTOP_TAGS) $(LINUX_TAGS)' -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(APP_NAME)-$(shell go env GOOS)-$(shell go env GOARCH) .
	@echo ">>> 完成: $(BUILD_DIR)/$(APP_NAME)-$(shell go env GOOS)-$(shell go env GOARCH)"

.PHONY: build-linux
build-linux: ## 构建 Linux 桌面版（Wails，需 webkit2gtk-4.0 & gtk+-3.0）
	@echo ">>> 构建 Linux 桌面版..."
	$(GO) build -tags '$(DESKTOP_TAGS) $(LINUX_TAGS)' -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 .
	@echo ">>> 完成: $(BUILD_DIR)/$(APP_NAME)-linux-amd64"

.PHONY: build-windows
build-windows: ## 交叉编译 Windows 桌面版（Wails/WebView2，需要 mingw-w64 + go-winres）
	@if ! command -v $(MINGW_CC) >/dev/null 2>&1; then \
		echo "错误: 未找到 $(MINGW_CC)。请先安装 mingw-w64:"; \
		echo "  Ubuntu/Debian: sudo apt install gcc-mingw-w64-x86-64"; \
		echo "  Arch/Manjaro:  sudo pacman -S mingw-w64-gcc"; \
		exit 1; \
	fi
	@if ! command -v $(WINRES) >/dev/null 2>&1; then \
		echo "错误: 未找到 $(WINRES)。请安装:"; \
		echo "  go install github.com/tc-hib/go-winres@latest"; \
		echo "  或将 WINRES 指向可执行文件路径"; \
		exit 1; \
	fi
	@echo ">>> 生成 Windows 资源（图标/版本信息/清单）..."
	$(WINRES) make --arch amd64 --file-version $(VERSION) --product-version $(VERSION)
	@echo ">>> 构建 Windows 桌面版（Wails/WebView2 交叉编译）..."
	# 注意：必须携带 Wails 框架的 production tag（否则走空壳实现，运行时报
	# "Wails applications will not build without the correct build tags"）
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=$(MINGW_CC) \
		$(GO) build -tags "$(DESKTOP_TAGS) windows" -ldflags "-H windowsgui $(LDFLAGS) -extldflags '-static'" \
		-o $(WIN_EXE) .
	@rm -f $(WINRES_SYSO)
	@echo ">>> 完成: $(WIN_EXE)"

.PHONY: build-cli
build-cli: ## 构建纯命令行版本（无 GUI 依赖，仅需纯 Go 工具链）
	@echo ">>> 构建 $(APP_NAME) 命令行版..."
	$(GO) build -tags $(CLI_TAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(APP_NAME)-cli-$(shell go env GOOS)-$(shell go env GOARCH) .
	@echo ">>> 完成: $(BUILD_DIR)/$(APP_NAME)-cli-$(shell go env GOOS)-$(shell go env GOARCH)"

.PHONY: build-cli-windows
build-cli-windows: ## 交叉编译 Windows 命令行版（无需 mingw-w64 / go-winres）
	@echo ">>> 构建 $(APP_NAME) Windows 命令行版..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
		$(GO) build -tags $(CLI_TAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(APP_NAME)-cli-windows-amd64.exe .
	@echo ">>> 完成: $(BUILD_DIR)/$(APP_NAME)-cli-windows-amd64.exe"

.PHONY: build-all
build-all: ## 构建所有平台版本（Linux + Windows 桌面版 + CLI）
	@$(MAKE) --no-print-directory build-linux
	@$(MAKE) --no-print-directory build-cli
	@$(MAKE) --no-print-directory build-cli-windows
	@$(MAKE) --no-print-directory build-windows
	@echo ">>> 全部构建完成！"

.PHONY: installer
installer: build-windows ## 生成 Windows 安装包（依赖 NSIS：sudo apt install nsis）
	@if ! command -v $(MAKENSIS) >/dev/null 2>&1; then \
		echo "错误: 未找到 $(MAKENSIS)。请先安装 NSIS:"; \
		echo "  Ubuntu/Debian: sudo apt install nsis"; \
		echo "  或将 MAKENSIS 指向可执行文件路径"; \
		exit 1; \
	fi
	@echo ">>> 生成 Windows 安装包..."
	# 将版本号规范化为纯数字 x.y.z.w（VIProductVersion 要求），非 x.y.z 格式回退 1.0.0.0
	FILEVERSION=$$(printf '%s' "$(VERSION)" | sed -E 's/^([0-9]+\.[0-9]+\.[0-9]+).*/\1.0/; t; s/^.*/1.0.0.0/') ; \
	cd packaging/nsis && $(MAKENSIS) -DVERSION=$(VERSION) -DFILEVERSION=$$FILEVERSION installer.nsi
	@mv -f packaging/nsis/$(INSTALLER_NAME) $(BUILD_DIR)/$(INSTALLER_NAME)
	@echo ">>> 完成: $(BUILD_DIR)/$(INSTALLER_NAME)"

.PHONY: build-installer
build-installer: installer ## 构建 Windows 可执行文件并打包安装程序

.PHONY: run
run: ## 运行程序
	@echo ">>> 运行 Wails 桌面版（需本机具备 Wails 运行时依赖）..."
	$(GO) run -tags '$(DESKTOP_TAGS) $(LINUX_TAGS)' .

.PHONY: test
test: ## 运行测试
	$(GO) test -tags '$(LINUX_TAGS)' ./...

.PHONY: fmt
fmt: ## 格式化代码
	$(GO) fmt ./...

.PHONY: vet
vet: ## 代码静态检查
	$(GO) vet -tags '$(LINUX_TAGS)' ./...

.PHONY: tidy
tidy: ## 整理依赖
	$(GO) mod tidy

.PHONY: clean
clean: ## 清理构建产物
	rm -rf $(BUILD_DIR)

.PHONY: list
list: ## 列出所有目标
	@$(MAKE) --no-print-directory help
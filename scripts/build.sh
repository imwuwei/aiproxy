#!/bin/bash
# AIProxy 构建脚本
# 用法: ./scripts/build.sh [windows|linux|all|cli|windows-cli|installer]
# 依赖: go 1.26+, mingw-w64 (gcc-mingw-w64-x86-64)
# Windows 构建额外依赖: go-winres（go install github.com/tc-hib/go-winres@latest）
# 桌面版基于 Wails v2 框架（已含于 go.mod）；Linux 下需 webkit2gtk-4.0 & gtk+-3.0
# 安装包构建额外依赖: nsis（sudo apt install nsis）

set -euo pipefail

cd "$(dirname "$0")/.."

BUILD_WINDOWS=0
BUILD_LINUX=0
BUILD_INSTALLER=0
BUILD_CLI=0
BUILD_WINDOWS_CLI=0
case "${1:-all}" in
  windows)     BUILD_WINDOWS=1 ;;
  linux)       BUILD_LINUX=1 ;;
  cli)         BUILD_CLI=1 ;;
  windows-cli) BUILD_WINDOWS_CLI=1 ;;
  all)         BUILD_WINDOWS=1; BUILD_LINUX=1; BUILD_CLI=1; BUILD_WINDOWS_CLI=1 ;;
  installer)   BUILD_WINDOWS=1; BUILD_INSTALLER=1 ;;
  *) echo "用法: $0 [windows|linux|all|cli|windows-cli|installer]"; exit 1 ;;
esac

mkdir -p build

# 桌面版（Wails）构建标签：production 为 Wails 框架必需（缺失时运行时报
# "Wails applications will not build without the correct build tags"）
DESKTOP_TAGS="production"

if [ "$BUILD_WINDOWS" = "1" ]; then
  if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
    echo "错误: 未找到 x86_64-w64-mingw32-gcc，请先安装 mingw-w64。"
    exit 1
  fi
  # 优先使用 PATH 中的 go-winres，否则回退到 GOPATH/bin
  WINRES="$(command -v go-winres 2>/dev/null || echo "$(go env GOPATH)/bin/go-winres")"
  if [ ! -x "$WINRES" ]; then
    echo "错误: 未找到 go-winres，请先安装: go install github.com/tc-hib/go-winres@latest"
    exit 1
  fi

  echo ">>> 生成 Windows 资源（图标/版本信息/清单）..."
  "$WINRES" make --arch amd64

  echo ">>> 构建 Windows 桌面版（Wails/WebView2 交叉编译）..."
  CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
    go build -tags "$DESKTOP_TAGS windows" -ldflags "-H windowsgui -s -w -extldflags '-static'" \
    -o build/aiproxy-windows-amd64.exe .
  rm -f rsrc_windows_amd64.syso
  echo ">>> 完成: build/aiproxy-windows-amd64.exe"
fi

if [ "$BUILD_LINUX" = "1" ]; then
  echo ">>> 构建 Linux 桌面版（Wails，需 webkit2gtk-4.0 & gtk+-3.0）..."
  go build -tags "$DESKTOP_TAGS x11" -ldflags "-s -w" -o build/aiproxy-linux-amd64 .
  echo ">>> 完成: build/aiproxy-linux-amd64"
fi

if [ "$BUILD_CLI" = "1" ]; then
  echo ">>> 构建命令行版（无 GUI 依赖）..."
  go build -tags cli -ldflags "-s -w" -o build/aiproxy-cli .
  echo ">>> 完成: build/aiproxy-cli"
fi

if [ "$BUILD_WINDOWS_CLI" = "1" ]; then
  echo ">>> 构建 Windows 命令行版（无需 mingw-w64 / go-winres）..."
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
    go build -tags cli -ldflags "-s -w" -o build/aiproxy-cli-windows-amd64.exe .
  echo ">>> 完成: build/aiproxy-cli-windows-amd64.exe"
fi

if [ "$BUILD_INSTALLER" = "1" ]; then
  if ! command -v makensis >/dev/null 2>&1; then
    echo "错误: 未找到 makensis，请先安装: sudo apt install nsis"
    exit 1
  fi

  VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo '1.0.0')"
  # 规范化版本号为纯数字 x.y.z.w（VIProductVersion 要求），非 x.y.z 格式回退 1.0.0.0
  FILEVERSION="$(printf '%s' "$VERSION" | sed -E 's/^([0-9]+\.[0-9]+\.[0-9]+).*/\1.0/; t; s/^.*/1.0.0.0/')"
  echo ">>> 生成 Windows 安装包（版本 $VERSION）..."
  (cd packaging/nsis && makensis "-DVERSION=$VERSION" "-DFILEVERSION=$FILEVERSION" installer.nsi)
  mv -f "packaging/nsis/aiproxy-Setup-$VERSION.exe" "build/aiproxy-Setup-$VERSION.exe"
  echo ">>> 完成: build/aiproxy-Setup-$VERSION.exe"
fi

echo ">>> 构建完成！"
#!/bin/bash
# 发布新版本脚本：同步 release-please 版本清单、打 tag 并推送 GitHub。
# .github/workflows/release.yml 在 tag 推送（v*）时自动构建全部平台产物并发布到 GitHub Release。
#
# 用法:
#   ./scripts/release.sh              # 自动在最新 tag 上递增 patch（v0.1.2 -> v0.1.3）
#   ./scripts/release.sh v0.2.0       # 指定版本号（形如 v1.2.3）
#
# 前置条件: 代码已提交、工作区干净；origin 远端已配置推送权限。
set -euo pipefail

cd "$(dirname "$0")/.."

# ---------- 1. 解析目标版本 ----------
VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  LATEST="$(git describe --tags --abbrev=0 2>/dev/null || echo 'v0.0.0')"
  VERSION="v$(printf '%s' "$LATEST" | sed -E 's/^v?([0-9]+)\.([0-9]+)\.([0-9]+).*/\1.\2.\3/' | awk -F. '{$3+=1; printf "%d.%d.%d", $1, $2, $3}')"
fi
case "$VERSION" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "错误: 版本号需形如 v1.2.3（当前: $VERSION）"; exit 1 ;;
esac

# ---------- 2. 前置校验 ----------
if [ -n "$(git status --porcelain)" ]; then
  echo "错误: 工作区有未提交的改动，请先提交:"
  git status --porcelain
  exit 1
fi
if git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null; then
  echo "错误: tag $VERSION 已存在"
  exit 1
fi

echo ">>> 发布版本: $VERSION"

# ---------- 3. 同步 release-please 版本清单（避免下次 main 推送时版本回跳） ----------
if [ -f .release-please-manifest.json ]; then
  python3 - "$VERSION" <<'EOF'
import json, sys
path = ".release-please-manifest.json"
with open(path, encoding="utf-8") as f:
    data = json.load(f)
data["."] = sys.argv[1].lstrip("v")
with open(path, "w", encoding="utf-8") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
EOF
  git add .release-please-manifest.json
  git commit -m "chore(release): bump manifest to $VERSION"
fi

# ---------- 4. 打 tag 并推送（触发 GitHub Actions 构建发布） ----------
git tag -a "$VERSION" -m "release: $VERSION"
git push origin HEAD
git push origin "$VERSION"

REMOTE="$(git config --get remote.origin.url || echo origin)"
echo ""
echo ">>> 已推送 $VERSION，GitHub Actions（release.yml）正在构建并发布 Release 资产。"
echo ">>> 构建进度与产物见仓库 Actions / Releases 页面。"

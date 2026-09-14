#!/usr/bin/env bash
# 交叉编译 cap 全平台 + 创建 GitHub release(需对 rayson-x/capnet 有写权限)
set -e
cd "$(dirname "$0")/.."
VERSION="${1:-v0.1.0}"
mkdir -p dist
for p in linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64; do
  out="dist/cap-${p%/*}-${p#*/}"
  [ "${p#*/}" = "amd64" ] && [ "${p%/*}" = "windows" ] && out="${out}.exe"
  GOOS=${p%/*} GOARCH=${p#*/} go build -ldflags="-s -w" -o "$out" ./cmd/cap
done
gh release create "$VERSION" dist/* --repo rayson-x/capnet \
  --title "capnet $VERSION" --notes "cap 单二进制:可扩展 node + 动态注册表 + 直连调用"
echo "release $VERSION created"

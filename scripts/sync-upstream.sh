#!/usr/bin/env bash
# 同步上游 Tencent/LoopForge 到本仓库的 main 分支。
#
#   scripts/sync-upstream.sh            # 拉取 upstream/main 并快进本地 main
#   scripts/sync-upstream.sh --push     # 同步后把 main 推送到 origin/main
#
# 分支约定：
#   main    持续跟随上游源码（只做镜像，不放个人改动）
#   dev     个人开发分支，从 main 拉出
#   stable  稳定发布分支，从 dev 打标签发布
set -euo pipefail

cd "$(dirname "$0")/.."

PUSH=0
for arg in "$@"; do
  case "$arg" in
    --push) PUSH=1 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

if ! git remote get-url upstream >/dev/null 2>&1; then
  echo "upstream remote missing; add it with:" >&2
  echo "  git remote add upstream https://github.com/Tencent/LoopForge.git" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "working tree is dirty; commit or stash first" >&2
  exit 1
fi

current="$(git rev-parse --abbrev-ref HEAD)"
if [ "$current" != "main" ]; then
  echo "checked out branch is '$current'; switching to main" >&2
  git checkout main
fi

echo "==> fetching upstream"
git fetch upstream main --tags

if ! git diff --quiet main upstream/main; then
  echo "==> upstream/main moved ahead; fast-forwarding main"
  git merge --ff-only upstream/main
else
  echo "==> main already matches upstream/main"
fi

if [ "$PUSH" -eq 1 ]; then
  echo "==> pushing main to origin"
  git push origin main
fi

echo
echo "main is at $(git rev-parse --short main) (upstream/main at $(git rev-parse --short upstream/main))"
echo "next: git checkout dev && git merge --no-ff main   # 把上游改动带入个人分支"

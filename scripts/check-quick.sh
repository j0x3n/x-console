#!/usr/bin/env bash
# 小修用的快速检查：只检查这次改动涉及的部分，完整检查交给 CI。
# 规则见 AGENTS.md 的“小修”。
#
# 用法（在仓库任意目录）：
#   scripts/check-quick.sh            # 和上游分支比，包括还没提交的改动
#   scripts/check-quick.sh HEAD~1     # 和指定的提交比
set -euo pipefail

root=$(git rev-parse --show-toplevel)
cd "$root"

if [ $# -gt 0 ]; then
  base=$1
elif git rev-parse --verify -q '@{upstream}' >/dev/null; then
  base=$(git merge-base HEAD '@{upstream}')
else
  base=HEAD
fi

changed=$({ git diff --name-only "$base"; git ls-files --others --exclude-standard; } | sort -u)
if [ -z "$changed" ]; then
  echo "没有改动，不用检查。"
  exit 0
fi

has() { grep -qE "$1" <<<"$changed"; }
step() { printf '\n== %s\n' "$*"; }
started=$(date +%s)

# ---------- 后端 ----------
if has '^(backend|api)/'; then
  cd "$root/backend"

  if has '^api/modules/|^backend/sqlc\.yaml$|queries\.sql$|/migrations/.*\.sql$'; then
    step "生成代码（接口定义或 SQL 有改动）"
    PATH=$PATH:$HOME/go/bin go generate ./...
  fi

  gofiles=$(grep -E '^backend/.*\.go$' <<<"$changed" | sed 's#^backend/##' | while read -r f; do
    [ -f "$f" ] && echo "$f"
  done || true)

  if [ -n "$gofiles" ]; then
    step "gofmt（改动的文件）"
    unformatted=$(gofmt -l $gofiles)
    if [ -n "$unformatted" ]; then
      echo "$unformatted"
      echo "上面的文件没格式化，运行 gofmt -w。"
      exit 1
    fi
  fi

  step "go build ./...（改动可能影响别的包，编译很快，全量编译）"
  go build ./...

  # 改动的包：模块目录下的改动测整个模块，其他的只测所在目录。
  # 迁移会让每个模块的 db/models.go 都重新生成，这些文件改了也不用把全部模块再测一遍
  # （上面的 go build ./... 已经检查它们能编译）。模块自己的 queries.sql.go 有改动才测那个模块。
  pkgs=$(grep -E '^backend/.*\.(go|sql)$' <<<"$changed" | grep -vE '/db/models\.go$' | sed 's#^backend/##' | while read -r f; do
    if [[ $f =~ ^(internal/server/modules/[^/]+)/ ]]; then
      echo "./${BASH_REMATCH[1]}/..."
    elif [[ $f == internal/server/store/migrations/* ]]; then
      echo "./internal/server/store/..."
    else
      d=$(dirname "$f")
      [ -d "$d" ] && echo "./$d"
    fi
  done | sort -u || true)

  if [ -n "$pkgs" ]; then
    # 本地默认不带 -race（一般要多花一倍时间），要带就 XC_RACE=1 scripts/check-quick.sh。
    # go test 自带的 vet 关掉，上一行已经单独跑过。
    step "go vet 和 go test（改动的包）"
    echo "$pkgs"
    go vet $pkgs
    go test -vet=off ${XC_RACE:+-race} $pkgs
  fi

  if has '^backend/(internal/agent|cmd/agent|pkg)/'; then
    step "Windows 代理交叉编译"
    GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/agent
    GOOS=windows GOARCH=amd64 go vet ./internal/agent/... ./cmd/agent
  fi
  cd "$root"
fi

# ---------- 前端 ----------
if has '^(web|api)/'; then
  cd "$root/web"

  if [ ! -d node_modules ] || has '^web/package(-lock)?\.json$'; then
    step "npm ci（依赖有变化或还没装）"
    npm ci
  fi

  if has '^api/modules/'; then
    step "生成前端接口类型"
    npm run gen:api
  fi

  step "类型检查"
  npm run typecheck

  srcfiles=$(grep -E '^web/src/.*\.(ts|tsx|css)$' <<<"$changed" | grep -v '^web/src/api/gen/' | sed 's#^web/##' | while read -r f; do
    [ -f "$f" ] && echo "$f"
  done || true)

  if [ -n "$srcfiles" ]; then
    step "prettier（改动的文件）"
    npx prettier --check $srcfiles

    tsfiles=$(grep -E '\.(ts|tsx)$' <<<"$srcfiles" || true)
    if [ -n "$tsfiles" ]; then
      step "vitest（和改动相关的测试）"
      npx vitest related --run --passWithNoTests $tsfiles
    fi
  fi

  if grep -qE '^web/src/.*\.(css|tsx)$' <<<"$changed"; then
    echo
    echo "提示：改了界面。只截改动的页面看一眼，比如 npm run shots -- --only /notes"
  fi
  cd "$root"
fi

echo
echo "快速检查通过，用时 $(($(date +%s) - started)) 秒。完整检查由 CI 跑（PR 和带部署标记的推送）。"

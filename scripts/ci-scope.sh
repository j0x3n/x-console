#!/usr/bin/env bash
# CI 用：看上次部署成功以来改了什么，决定这次要跑哪些测试。
#
# 用法：scripts/ci-scope.sh <基准提交>      基准为空时全部都跑
# 输出（key=value，直接追加到 $GITHUB_OUTPUT）：
#   backend  是否跑后端检查（生成代码是否最新、gofmt、vet、Windows 编译）
#   web      是否跑前端检查和单元测试
#   tests    后端单元测试：full 全部（分 8 组）、some 只测下面的 packages、none 不测
#   shards   后端测试矩阵，full 是 [1..8]，其余是 [1]
#   packages tests=some 时要测的包，空格分隔
# 端到端测试（e2e）不看这里，每次都跑。
#
# 判断规则（2026-10-11，用户让只测改到的部分）：
#   - 只有 backend/internal/server/modules/<模块>/ 和 api/modules/<模块>.yaml 里的改动，
#     才算“只改了这个模块”，只测这些模块，再加上下面 COUPLED 里几个会碰到别的模块的包。
#   - 各模块的 db/models.go 是生成的，加迁移时每个模块都会变，不算改动。
#   - 新迁移只加测 internal/server/store。
#   - 其他任何后端改动、接口公共定义、依赖、脚本、工作流，甚至认不出的路径，一律全量。
#   - 只有 docs 和 .md 的改动什么都不跑。
set -euo pipefail

base=${1:-}
cd "$(git rev-parse --show-toplevel)"

# 这些包的测试会走到别的模块（模块列表、隐藏内容、活动来源、动作目录等），任何模块有改动都要测
COUPLED="./internal/server/app/... ./internal/server/core/... ./internal/server/actions/... ./internal/server/modules/journal/... ./internal/server/modules/vault/... ./internal/server/modules/dashboard/... ./internal/server/modules/mcp/... ./internal/server/modules/ai/..."

emit() { printf '%s=%s\n' "$1" "$2"; }
full() {
  emit backend true; emit web true; emit tests full; emit shards '[1,2,3,4,5,6,7,8]'; emit packages ''
  exit 0
}

[ -n "$base" ] || full
files=$(git diff --name-only "$base" HEAD 2>/dev/null) || full
printf '%s\n' "$files" >&2

backend=false web=false store=false
mods=()
while IFS= read -r f; do
  [ -n "$f" ] || continue
  case "$f" in
    docs/*|*.md) ;;
    backend/internal/server/modules/*/db/models.go) backend=true ;;
    backend/internal/server/modules/*/*)
      m=${f#backend/internal/server/modules/}; m=${m%%/*}
      mods+=("$m"); backend=true ;;
    backend/internal/server/store/migrations/*) store=true; backend=true ;;
    api/modules/common.yaml|api/modules/vault.yaml) full ;;
    api/modules/*.yaml)
      m=$(basename "$f" .yaml)
      [ -d "backend/internal/server/modules/$m" ] || full
      mods+=("$m"); backend=true; web=true ;;
    web/*) web=true ;;
    *) full ;;
  esac
done <<<"$files"

if [ "$backend" = false ]; then
  emit backend false; emit web "$web"; emit tests none; emit shards '[1]'; emit packages ''
  exit 0
fi

pkgs=$COUPLED
[ "$store" = true ] && pkgs="$pkgs ./internal/server/store/..."
for m in $(printf '%s\n' "${mods[@]:-}" | sort -u); do
  [ -n "$m" ] && pkgs="$pkgs ./internal/server/modules/$m/..."
done
emit backend true; emit web "$web"; emit tests some; emit shards '[1]'
emit packages "$(printf '%s\n' $pkgs | sort -u | tr '\n' ' ' | sed 's/ $//')"

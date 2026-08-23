#!/usr/bin/env bash
# scripts/check_file_length.sh — 单文件行数门禁
#
# 对标 README 规范「单文件 ~400 行主动拆分」。覆盖后端非测试/非生成 .go 与前端 src .vue/.ts。
#
# 棘轮(ratchet)语义,保证 CI 始终绿:
#   - 非登记文件 > THRESHOLD  → 失败(新增超标)
#   - 登记文件当前行数 > baseline 记录值 → 失败(只许缩不许涨)
#   - 其余通过
# baseline 即当前超标清单,只会随拆分收缩;收紧阈值或清理后重跑 init 刷新。
#
# 用法:
#   bash scripts/check_file_length.sh check [threshold]   # 跑门禁(默认阈值 400)
#   bash scripts/check_file_length.sh init  [threshold]   # 用当前超标文件覆盖 baseline
set -euo pipefail

MODE="${1:-check}"
THRESHOLD="${2:-400}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASELINE="$DIR/scripts/file_length_baseline.txt"

# 输出 "lines\tpath"(path 相对仓库根)。单次 wc 批量取行数,避免逐文件子进程。
# git ls-files 只列被跟踪文件——本地未跟踪/被 ignore 的文件不参与门禁
# (历史坑:曾被 gitignore 的 frontend/src/data/*.ts 本地存在时被 find 扫到,
#  误报 NEW OFFENDER;未入库的文件门禁管不到也不该管)。
scan() {
  ( cd "$DIR"
    git ls-files -- 'backend/*.go' 'frontend/src/*.vue' 'frontend/src/*.ts' \
      | grep -vE '(_test\.go$|\.pb\.go$|/zz_[^/]*\.go$|backend/vendor/)' \
      | tr '\n' '\0' | xargs -0 wc -l 2>/dev/null \
      | awk 'NF==2 && $2!="total"{print $1"\t"$2}'
  )
}

case "$MODE" in
  init)
    scan | awk -F'\t' -v t="$THRESHOLD" '$1+0>t+0{print}' | sort -t$'\t' -k2 > "$BASELINE"
    echo "baseline written: $(wc -l < "$BASELINE") file(s) > $THRESHOLD lines -> scripts/file_length_baseline.txt"
    ;;
  check)
    [[ -f "$BASELINE" ]] || { echo "FATAL: 无 baseline,先跑 'bash scripts/check_file_length.sh init'"; exit 1; }
    declare -A BASE=()
    while IFS=$'\t' read -r lines path; do
      [[ -n "$path" ]] && BASE["$path"]="$lines"
    done < "$BASELINE"

    fail=0
    while IFS=$'\t' read -r lines path; do
      [[ -n "$path" ]] || continue
      if (( lines > THRESHOLD )); then
        cap="${BASE[$path]:-}"
        if [[ -z "$cap" ]]; then
          echo "NEW OFFENDER: $path = ${lines} 行 (> ${THRESHOLD}, 未登记 baseline)"; fail=1
        elif (( lines > cap )); then
          echo "REGRESSION: $path 由 ${cap} 涨到 ${lines} 行(只许缩不许涨)"; fail=1
        fi
      fi
    done < <(scan)
    exit $fail
    ;;
  *)
    echo "usage: bash $0 {check|init} [threshold=400]"; exit 2 ;;
esac

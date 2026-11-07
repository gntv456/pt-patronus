#!/usr/bin/env bash
# scripts/check_file_length.sh — 单文件行数/列宽门禁
#
# 对标 README 规范「单文件 ~400 行主动拆分；行宽 ≤80 列」。覆盖后端非测试/非生成
# .go、前端 src .vue/.ts 与移动端 mobile/lib .dart（不含 .g.dart 生成代码）。
#
# 棘轮(ratchet)语义,保证 CI 始终绿:
#   - 非登记文件 > THRESHOLD  → 失败(新增超标)
#   - 登记文件当前行数 > baseline 记录值 → 失败(只许缩不许涨)
#   - 其余通过
# baseline 即当前超标清单,只会随拆分收缩;收紧阈值或清理后重跑 init 刷新。
#
# 80 列口径：**字符数**而非字节——本仓大量中文文案，UTF-8 下每个汉字 3 字节，
# awk 的 length() 按字节计会把 40 个汉字的行误报成 120 列，故必须按码点数。
# 例外：URL/长标识符无法折行，登记后按「只许减不许增」棘轮收缩，与行数同款。
#
# 用法:
#   bash scripts/check_file_length.sh check [threshold]        # 跑门禁(默认行 400/列 80)
#   bash scripts/check_file_length.sh init  [threshold]        # 用当前超标文件覆盖 baseline
#   WIDTH_ONLY=1 bash …  /  LINES_ONLY=1 bash …                # 只跑其中一项
set -euo pipefail

MODE="${1:-check}"
THRESHOLD="${2:-400}"
COLS="${COLS:-80}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASELINE="$DIR/scripts/file_length_baseline.txt"
COLS_BASELINE="$DIR/scripts/file_cols_baseline.txt"

# 输出 "lines\tpath"(path 相对仓库根)。单次 wc 批量取行数,避免逐文件子进程。
# git ls-files 只列被跟踪文件——本地未跟踪/被 ignore 的文件不参与门禁
# (历史坑:曾被 gitignore 的 frontend/src/data/*.ts 本地存在时被 find 扫到,
#  误报 NEW OFFENDER;未入库的文件门禁管不到也不该管)。
# 数据类文件(应用商店模板/i18n 翻译表)是纯数据——行数不反映逻辑复杂度，单独阈值。
DATA_THRESHOLD="${DATA_THRESHOLD:-2000}"

scan() {
  ( cd "$DIR"
    git ls-files -- 'backend/*.go' 'frontend/src/*.vue' 'frontend/src/*.ts' 'mobile/lib/*.dart' \
      | grep -vE '(_test\.go$|\.pb\.go$|/zz_[^/]*\.go$|backend/vendor/|\.g\.dart$)' \
      | tr '\n' '\0' | xargs -0 wc -l 2>/dev/null \
      | awk -v t="$THRESHOLD" -v dt="$DATA_THRESHOLD" '
        NF==2 && $2!="total" {
          n=$1+0; path=$2
          lim = (path ~ /^frontend\/src\/(data|i18n\/locales)\//) ? dt : t
          if (n > lim+0) print n"\t"path
        }'
  )
}

# scan_cols: 每个超标文件输出 "超宽行数\tpath"。用 python 按字符数计宽
# （见文件头「80 列口径」说明）；git ls-files 语义与 scan() 一致。
scan_cols() {
  ( cd "$DIR"
    git ls-files -- 'backend/*.go' 'frontend/src/*.vue' 'frontend/src/*.ts' 'mobile/lib/*.dart' \
      | grep -vE '(_test\.go$|\.pb\.go$|/zz_[^/]*\.go$|backend/vendor/|\.g\.dart$)' \
      | COLS="$COLS" python -c '
import os, sys
cols = int(os.environ["COLS"])
# strip() 同时剥行尾 CR：Windows 管道下 stdin 行保留 CR，path 带尾 CR 会与
# baseline key 不匹配（CI 上无此问题，本地 Git Bash 有；1.03.27 排查记录）。
import sys as _s
_w = _s.stdout.buffer.write  # Windows 文本模式 print 会补 CR，改二进制写 LF
for path in (x.strip() for x in _s.stdin.read().split()):
    try:
        n = 0
        with open(path, encoding="utf-8", errors="replace") as f:
            for line in f:
                if len(line.rstrip("\r\n")) > cols:
                    n += 1
        if n:
            _w(f"{n}\t{path}\n".encode())
    except OSError:
        pass
'
  )
}

run_lines_check() {
  [[ -f "$BASELINE" ]] || { echo "FATAL: 无 baseline,先跑 'bash scripts/check_file_length.sh init'"; exit 1; }
  declare -A BASE=()
  # 读 baseline 前剥 CR 与 BOM：baseline 在 Windows 端可能被 CRLF 工具改写
  # （1.03.27 CI 事故根因：read 保留 CR，key 带 CR 与 scan 输出不匹配 → 全量 NEW WIDE）。
  while IFS=$'\t' read -r lines path; do
    path="${path%$'\r'}"
    [[ -n "$path" ]] && BASE["$path"]="$lines"
  done < <(sed '1s/^\xef\xbb\xbf//' "$BASELINE" | tr -d '\r')

  fail=0
  while IFS=$'\t' read -r lines path; do
    [[ -n "$path" ]] || continue
    lim=THRESHOLD
    case "$path" in frontend/src/data/*|frontend/src/i18n/locales/*) lim=DATA_THRESHOLD;; esac
    if (( lines > lim )); then
      cap="${BASE[$path]:-}"
      if [[ -z "$cap" ]]; then
        echo "NEW OFFENDER: $path = ${lines} 行 (> ${lim}, 未登记 baseline)"; fail=1
      elif (( lines > cap )); then
        echo "REGRESSION: $path 由 ${cap} 涨到 ${lines} 行(只许缩不许涨)"; fail=1
      fi
    fi
  done < <(scan)
  return $fail
}

run_cols_check() {
  [[ -f "$COLS_BASELINE" ]] || { echo "FATAL: 无列宽 baseline($COLS_BASELINE),先跑 'bash scripts/check_file_length.sh init'"; exit 1; }
  declare -A BASE=()
  # 同 run_lines_check：剥 CR 与 BOM，防 Windows 端 CRLF 改写让 CI 全量 NEW WIDE。
  while IFS=$'\t' read -r cnt path; do
    path="${path%$'\r'}"
    [[ -n "$path" ]] && BASE["$path"]="$cnt"
  done < <(sed '1s/^\xef\xbb\xbf//' "$COLS_BASELINE" | tr -d '\r')

  fail=0
  while IFS=$'\t' read -r cnt path; do
    path="${path%$'\r'}"; cnt="${cnt%$'\r'}"
    [[ -n "$path" ]] || continue
    cap="${BASE[$path]:-}"
    if [[ -z "$cap" ]]; then
      echo "NEW WIDE: $path 有 ${cnt} 行超过 ${COLS} 列(未登记 cols baseline)"; fail=1
    elif (( cnt > cap )); then
      echo "WIDER: $path 超宽行由 ${cap} 涨到 ${cnt}(只许减不许增)"; fail=1
    fi
  done < <(scan_cols)
  return $fail
}

case "$MODE" in
  init)
    scan | sort -t$'\t' -k2 > "$BASELINE"
    scan_cols | sort -t$'\t' -k2 > "$COLS_BASELINE"
    echo "baseline written: $(wc -l < "$BASELINE") file(s) > $THRESHOLD lines (data: $DATA_THRESHOLD) -> scripts/file_length_baseline.txt"
    echo "cols baseline written: $(wc -l < "$COLS_BASELINE") file(s) 含 >${COLS} 列行 -> scripts/file_cols_baseline.txt"
    ;;
  check)
    rc=0
    if [[ "${LINES_ONLY:-0}" != "1" ]]; then run_lines_check || rc=1; fi
    if [[ "${WIDTH_ONLY:-0}" != "1" ]]; then run_cols_check || rc=1; fi
    exit $rc
    ;;
  *)
    echo "usage: bash $0 {check|init} [threshold=400]"; exit 2 ;;
esac

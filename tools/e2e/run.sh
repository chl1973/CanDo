#!/usr/bin/env bash
# 端到端浏览器测试：启动模拟模型 + 测试版工作台（全新数据目录），跑 Playwright 脚本，截图在 tools/e2e/out/
# 依赖：Go、Python3、pip install playwright && python -m playwright install chromium
set -euo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)
TMP=$(mktemp -d)
go build -tags e2e -o "$TMP/cando_e2e" "$ROOT"
python3 fake_llm.py 11434 > "$TMP/fake.log" 2>&1 & FAKE=$!
cleanup(){ kill $FAKE ${S1:-} ${S2:-} ${S3:-} ${S4:-} 2>/dev/null || true; }
trap cleanup EXIT
mkdir -p "$TMP/d1" "$TMP/d2" "$TMP/d3" "$TMP/d4"
"$TMP/cando_e2e" -data "$TMP/d1" -port 18820 -no-browser > "$TMP/s1.log" 2>&1 & S1=$!
"$TMP/cando_e2e" -data "$TMP/d2" -port 18817 -no-browser > "$TMP/s2.log" 2>&1 & S2=$!
"$TMP/cando_e2e" -data "$TMP/d3" -port 18824 -no-browser > "$TMP/s3.log" 2>&1 & S3=$!
"$TMP/cando_e2e" -data "$TMP/d4" -port 18825 -no-browser > "$TMP/s4.log" 2>&1 & S4=$!
sleep 2
echo "== 主流程（首次设置、首页、论文库、AI 起草、深色模式、手机导航）"; python3 e2e_main.py http://127.0.0.1:18820
echo "== 后台任务"; python3 e2e_jobs.py http://127.0.0.1:18817
echo "== 论文契约"; python3 e2e_contract.py http://127.0.0.1:18824
echo "== 本机智能体 × Office"; python3 e2e_office.py http://127.0.0.1:18825

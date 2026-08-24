#!/usr/bin/env bash
# Tobias 25.08. ~00:20: Gemini 3.7 Flash via Antigravity-Abo auf effort=LOW durch die komplette
# 8-Task-Suite, danach Token-Bilanz. Start verzoegert (Default 8100 s = 2 h 15 min), damit das
# 5-Stunden-Quota-Fenster frisch ist. Label agy-37flash-low (U1/A1-Stichprobe von 23:1x wird
# durch den vollen Lauf ueberholt). Ergebnis: results.jsonl + bench/agy-low-tokens.md.
# Usage: agy-low-suite.sh [delay-seconds]
DELAY="${1:-8100}"
cd "$HOME/ai-lab/bench" || exit 1
echo "=== $(date +%H:%M) warte ${DELAY}s bis zum Quota-Reset"; sleep "$DELAY"
export AGY_MODEL=gemini-3.7-flash AGY_EFFORT=low
echo "=== $(date +%H:%M) Start agy-37flash-low Suite (parallel evtl. lokale Laeufe auf der GPU — Wandzeiten leicht konfundiert)"
for t in aiux-U1-paging agora-A1-gate agora-A2-jsonld agora-A3-hls aiux-U2-denytools agora-A4-feed agora-A5-batcher-scratch agora-A6-scorer-scratch; do
  lim=900; case "$t" in aiux-U2-denytools|agora-A5-*|agora-A6-*) lim=1800 ;; esac
  ./run-task-agy.sh agy-37flash-low "$t" "$lim"
done
echo "=== $(date +%H:%M) Token-Bilanz"
python3 - <<'EOF' | tee "$HOME/ai-lab/bench/agy-low-tokens.md"
import json,glob,os
os.chdir(os.path.expanduser('~/ai-lab/bench'))
def usage(f):
    try: u=json.load(open(f)).get('usage') or {}
    except Exception: return None
    return {k:u.get(k,0) or 0 for k in ('input_tokens','output_tokens','thinking_tokens','cache_read_tokens','total_tokens')} if u else None
print("# agy-37flash-low — Token-Bilanz Suite (8 Tasks)\n")
print("| Task | Input | Output | davon Thinking | Cache-Reads | total |\n|---|---|---|---|---|---|")
tot={}
for f in sorted(glob.glob('runs/agy-37flash-low/*/transcript.json')):
    u=usage(f)
    if not u: print(f"| {f.split('/')[1]} | (kein Envelope) | | | | |"); continue
    for k,v in u.items(): tot[k]=tot.get(k,0)+v
    print(f"| {f.split('/')[1]} | {u['input_tokens']:,} | {u['output_tokens']:,} | {u['thinking_tokens']:,} | {u['cache_read_tokens']:,} | {u['total_tokens']:,} |")
if tot: print(f"| **gesamt** | **{tot['input_tokens']:,}** | **{tot['output_tokens']:,}** | {tot['thinking_tokens']:,} | {tot['cache_read_tokens']:,} | **{tot['total_tokens']:,}** |")
print("\nVergleich high (Suite 8 Tasks, 24.08.): Input 2.091.183 · Output 169.773 · Thinking 108.930 · Cache 14.539.411 · total 2.260.956")
EOF
echo "AGY_LOW_SUITE_KOMPLETT"

#!/usr/bin/env bash
# Kandidaten-Runde 100-130B-MoE (Tobias 24.08. ~23:15, Auswahl durch Claude ~23:30):
# drei Modelle in derselben Quant-Klasse (imatrix IQ3_XXS, 42-46 GiB) laden.
# Warum IQ3_XXS: kleinste Stufe, die ins ~45-GiB-Budget (20 VRAM + 32 RAM - OS/KV) passt;
# alles darueber streamt Experten von NVMe (tg bricht auf 1-3 t/s ein). Unsloth-UD hat
# fuer alle drei nichts unter Q3_K_M (~50 GiB) -> mradermacher/bartowski.
# Fallback bei OOM/Streaming: IQ2_M (~37 GiB) derselben Quantizer (Zeile unten einkommentieren).
#
# Schutz des laufenden 80B-Polyglots: aria2c laeuft in einer cgroup mit MemoryHigh=1G,
# damit sein Schreib-Page-Cache nicht die mmap-Expertenseiten von llama-server verdraengt.
# Fortsetzbar (aria2c -c). Erfolgsmarker: models/.kandidaten-download-ok
set -uo pipefail
M="$HOME/ai-lab/models"; L="$HOME/ai-lab/logs"; mkdir -p "$M" "$L"
LOG="$L/kandidaten-download.log"
log() { printf '%s %s\n' "$(date '+%d.%m %H:%M:%S')" "$*" | tee -a "$LOG"; }

# name|repo|datei
CANDS=(
  "mistral4|mradermacher/Mistral-Small-4-119B-2603-i1-GGUF|Mistral-Small-4-119B-2603.i1-IQ3_XXS.gguf"
  "qwen35|mradermacher/Qwen3.5-122B-A10B-i1-GGUF|Qwen3.5-122B-A10B.i1-IQ3_XXS.gguf"
  "laguna|bartowski/Laguna-S-2.1-GGUF|Laguna-S-2.1-IQ3_XXS.gguf"
  # Fallbacks (IQ2_M): bei Bedarf einkommentieren
  # "mistral4-iq2|mradermacher/Mistral-Small-4-119B-2603-i1-GGUF|Mistral-Small-4-119B-2603.i1-IQ2_M.gguf"
  # "qwen35-iq2|mradermacher/Qwen3.5-122B-A10B-i1-GGUF|Qwen3.5-122B-A10B.i1-IQ2_M.gguf"
  # "laguna-iq2|bartowski/Laguna-S-2.1-GGUF|Laguna-S-2.1-IQ2_M.gguf"
)

ok=0; fail=0
for c in "${CANDS[@]}"; do
  name="${c%%|*}"; rest="${c#*|}"; repo="${rest%%|*}"; file="${rest#*|}"
  url="https://huggingface.co/$repo/resolve/main/$file"
  expected=$(curl -sIL "$url" | grep -i '^content-length:' | tail -1 | tr -dc '0-9')
  log "== $name: $file (erwartet $((expected/1073741824)) GiB) von $repo"
  if [ -f "$M/$file" ] && [ -n "$expected" ] && [ "$(stat -c %s "$M/$file")" = "$expected" ]; then
    log "   bereits vollstaendig vorhanden"; ok=$((ok+1)); continue
  fi
  systemd-run --user --scope -q -p MemoryHigh=1G -p MemoryMax=2G \
    aria2c -x 8 -s 8 -k 4M -c --file-allocation=none --disk-cache=0 \
      --console-log-level=warn --summary-interval=120 \
      -d "$M" -o "$file" "$url" >> "$LOG" 2>&1
  rc=$?
  actual=$(stat -c %s "$M/$file" 2>/dev/null || echo 0)
  if [ "$rc" -eq 0 ] && { [ -z "$expected" ] || [ "$actual" = "$expected" ]; }; then
    log "   OK ($((actual/1073741824)) GiB)"; ok=$((ok+1))
  else
    log "   !! FEHLER rc=$rc (ist $((actual/1073741824)) GiB, erwartet $((expected/1073741824)) GiB)"; fail=$((fail+1))
  fi
done
log "== fertig: $ok ok, $fail fehlgeschlagen"
[ "$fail" -eq 0 ] && touch "$M/.kandidaten-download-ok"
echo "KANDIDATEN-DOWNLOAD KOMPLETT ok=$ok fail=$fail"

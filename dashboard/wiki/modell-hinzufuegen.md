# Neues lokales Modell hinzufügen

**Frage dieser Seite: Wie kommt ein neues GGUF von Hugging Face bis zur Hero-Karte im Dashboard?**

## 1. GGUF wählen — Quant-Faustregeln für 20 GB VRAM

Budget: Modell + KV-Cache (bei 32k Kontext, q8_0) müssen unter ~19–20 GB bleiben. Kalibriert an den vier Lab-Modellen:

| Modellgröße | Quant, die hier funktioniert | Beleg aus dem Lab |
|---|---|---|
| bis ~30B dicht | Q4-Klasse (IQ4_XS, Q4_K_XL) | Qwen3.8-27B IQ4_XS = 13,3 GiB; Muse-30B Q4_K_XL = 15 GiB — beide komplett in VRAM inkl. 32k KV |
| ~35B MoE | Q3_K_XL | Qwen3.6-35B-A3B = 15,7 GiB, komplett in VRAM |
| deutlich größer (nur MoE) | Q3_K_XL + Experten in den RAM | Qwen3-Coder-Next 80B = 33,8 GiB, läuft mit `--n-cpu-moe 26` unter ~19 GB VRAM |
| Vision-Variante | mmproj kostet extra (0,9–2 GiB) | deshalb fahren die Vision-Zweige nur 16k Kontext |

Unsloth-„UD"-Quants haben sich bewährt (alle vier Lab-Modelle). Passt es knapp nicht: kleinere Quant **oder** weniger Kontext, nicht beides raten — [Server & Flags](server-und-flags.md) hat die Kontext-vs-VRAM-Tabelle.

## 2. Download (CLI)

Muster aus `download-models.sh` (aria2c, resümierbar):

```bash
cd ~/ai-lab/models
aria2c -x8 -s8 -c --file-allocation=falloc \
  -o "<datei>.gguf" "https://huggingface.co/<repo>/resolve/main/<datei>.gguf"
# oder:
huggingface-cli download <repo> <datei>.gguf --local-dir ~/ai-lab/models
```

## 3. Registry: serve.sh-Zweig

Neuer `case`-Zweig in `~/ai-lab/serve.sh`, mit den Lab-Standardflags (stecken in `COMMON`):

```bash
  meinmodell)
    exec llama-server "${COMMON[@]}" \
      -m "$M/MeinModell.gguf" \
      -ngl 99 -c 32768 -ctk q8_0 -ctv q8_0 "$@" ;;
```

`--jinja` ist Pflicht (Tool-Calls!) und steckt schon in `COMMON`. Für große MoE zusätzlich `--n-cpu-moe <n>` (Vorbild `codernext`-Zweig).

## 4. Erster Test

```bash
~/ai-lab/serve.sh meinmodell vulkan
curl -s http://127.0.0.1:8080/v1/models -H 'Authorization: Bearer sk-local'
```

Dann ein Suite-Task — **via App:** Dashboard (`:8100`) → Läufe-Tab → „Suite — run-task.sh", Modell + Task wählen (das Dropdown kennt nur Modelle aus `MODELS`, siehe Schritt 5; vorher geht der CLI-Weg). **Via CLI:**

```bash
~/ai-lab/bench/run-task.sh meinmodell-vulkan agora-A1-gate
```

Wichtig: `run-task.sh` benutzt, was auf `:8080` läuft — das Label ist nur Beschriftung. Falscher Server = falsch beschriftete Zahlen.

## 5. Dashboard-Registry

Neue Labels tauchen in der Suite-Matrix automatisch auf. Für eine eigene Hero-Karte in der Übersicht: in `dashboard/server.py` zwei Zeilen ergänzen —

```python
MODELS = ['qwen38', 'qwen36moe', 'muse', 'codernext', 'meinmodell']
MODEL_META = { ... 'meinmodell': {'name': 'Mein Modell 32B', 'note': 'Q4 · dicht'}, }
```

## 6. Vermessen

In dieser Reihenfolge:

1. **Suite:** alle 5 Tasks (Läufe-Tab oder Schleife über `run-task.sh`).
2. **Perf:** `~/ai-lab/perf-bench.sh` um einen Zweig ergänzen — **nie parallel zum laufenden Server** (gleiche GPU). Ergebnis: `logs/perf-*.json` → Perf-Tab.
3. **Polyglot:** Server starten, dann `~/ai-lab/bench/aider/run-polyglot-subset.sh`. Danach den neuen Lauf in `dashboard/polyglot-labels.json` dem Modell zuordnen — die Run-Namen tragen das Modell nicht, ohne Zuordnung bleibt die Hero-Karte ohne Polyglot-Wert.

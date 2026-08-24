# Server & Flags

**Frage dieser Seite: Wie starte ich den Modell-Server richtig — und warum stehen diese Flags in serve.sh?**

```bash
~/ai-lab/serve.sh <modell> [vulkan|rocm] [extra llama-server-Args...]
~/ai-lab/serve.sh qwen38 vulkan            # Standard
~/ai-lab/serve.sh qwen38 vulkan -c 65536   # Extra-Args gehen direkt an llama-server
```

Lauscht auf `http://127.0.0.1:8080`, API-Key `sk-local`, spricht OpenAI- **und** Anthropic-API. Modelle: `qwen38`, `qwen38-vision`, `qwen36moe`, `muse`, `muse-vision`, `codernext`.

## Die 6 wichtigen Flags — und ihr Warum

| Flag | Warum |
|---|---|
| `--jinja` | **Pflicht.** Aktiviert das Chat-Template inkl. Tool-Call-Grammatik. Ohne läuft z. B. Qwen3.8 über seine Stop-Tokens hinaus — Agenten funktionieren dann schlicht nicht. |
| `--cache-reuse 256` | Prompt-Präfix-Wiederverwendung zwischen Agent-Turns. Agenten schicken denselben Verlauf immer wieder — das ist der wichtigste einzelne Latenz-Hebel für Agent-Workloads. |
| `-ctk q8_0 -ctv q8_0` | KV-Cache auf 8 Bit: **halber VRAM für den Kontext**, vernachlässigbarer Qualitätsverlust. Deshalb passen 32k Kontext neben ein 15-GiB-Modell. |
| `-c <n>` | Kontextfenster. 32k Default, Vision-Zweige 16k (mmproj kostet VRAM). Mehr Kontext = mehr KV-VRAM; pro Lauf überschreibbar. Der 27B-Polyglot-Lauf zeigt, was zu wenig kostet: 17× Kontext erschöpft ([Troubleshooting](troubleshooting.md)). |
| `--n-cpu-moe <n>` | Nur `codernext`: n Routed-Expert-Schichten bleiben per mmap im RAM, Attention + Shared Experts auf der GPU. So läuft ein 34-GiB-GGUF auf einer 20-GB-Karte (26 hält den VRAM unter ~19 GB). Zu klein gewählt = VRAM-Übersubscription. |
| `-t 8` | Physische Kerne des 7800X3D, nicht SMT-Threads — SMT bringt bei GGML-Matmuls nichts. |

Fest gesetzt außerdem: `-fa on` (Flash Attention), `-ngl 99` (alle Layer auf die GPU), `--metrics`.

## Kontext vs. VRAM

| Modell | GGUF | Kontext | VRAM |
|---|---|---|---|
| `qwen38` (27B dicht) | 13,3 GiB | 32k | komplett in VRAM, ~15–16 GB inkl. KV |
| `qwen38-vision` | +0,9 GiB mmproj | 16k | komplett in VRAM |
| `qwen36moe` (35B-A3B) | 15,7 GiB | 32k | komplett in VRAM |
| `muse` (30B) | 15 GiB | 32k | komplett in VRAM |
| `muse-vision` | +2 GiB mmproj | 16k | komplett in VRAM |
| `codernext` (80B-A3B) | 33,8 GiB | 32k | VRAM + RAM (`--n-cpu-moe 26`, < ~19 GB VRAM) |

## Vulkan vs. ROCm — der Befund

llama-bench-Matrix (pp2048 / tg128 bei Kontexttiefe 0 / 10k / 32k), Zahlen in tok/s aus `logs/perf-*.json`:

| Modell | tg128 @0 (V / R) | tg128 @32k (V / R) | pp2048 @0 (V / R) |
|---|---|---|---|
| qwen36moe | **122** / 97 | **109** / 77 | 2472 / 2308 |
| muse | **38** / 33 | **37** / 30 | 756 / **906** |
| qwen38 | 33* / 33 | **32** / 26 | 476* / 828 |

\* qwen38-Vulkan: nur die 32k-Messung liegt vollständig vor (`perf-qwen38-vulkan-d32.json`); der volle Matrix-Lauf ist unvollständig (`-partial.json`).

**Kurz:** Vulkan gewinnt die Token-Generierung auf allen Modellen und hält das Tempo bei tiefem Kontext deutlich besser (35B: +42 % bei 32k). ROCm gewinnt bei Muse das Prompt-Processing. Da Agent-Latenz von Generierung bei tiefem Kontext dominiert wird, ist **Vulkan der Default** — ROCm bleibt pro Lauf wählbar. Für `codernext` liegen keine Perf-JSONs vor [unverifiziert/nicht gemessen].

**Nie `perf-bench.sh` parallel zum laufenden Server** — gleiche GPU, beide Messungen werden Müll.

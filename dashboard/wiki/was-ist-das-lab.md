# Was ist das Lab?

**Frage dieser Seite: Aus welchen Schichten besteht das Lab, und warum gibt es jede davon?**

Das Lab betreibt Coding-Agenten mit lokalen LLMs auf einer einzigen Consumer-GPU — und misst sie objektiv. Cloud-APIs (DeepSeek, Gemini) docken über dieselbe Infrastruktur nur als Vergleichswerte an.

**Hardware:** Ryzen 7 7800X3D · 32 GB DDR5 · RX 7900 XT mit 20 GB VRAM (gfx1100) · NVMe · CachyOS.

## Die fünf Schichten

```
Modell-Server ──► Harness ──► Sandbox ──► Grader ──► Dashboard
 (llama-server)   (OpenCode)   (podman)    (Host)     (:8100)
```

**1. Modell-Server** — `serve.sh` startet `llama-server` (llama.cpp aus pacman) auf `http://127.0.0.1:8080`, API-Key `sk-local`. Er spricht die OpenAI- **und** die Anthropic-API und serviert genau ein Modell zur Zeit. *Warum eine eigene Schicht:* Jedes Werkzeug (OpenCode, aider, Claude Code, die DOM/UX-Skripte) redet nur mit diesem einen Endpoint — Modell tauschen heißt Server neu starten, sonst ändert sich nichts.

**2. Harness** — der Agent, der das Modell arbeiten lässt. Standard ist **OpenCode** (Provider `llamacpp` → `:8080/v1`); daneben gibt es **dsh** (DeepSeeks eigenes CLI, nur für DeepSeek-Vergleiche), **aider** (Polyglot-Benchmark) und kleine Python-Loops (`dom-agent.py`, `ux-dom-test.py`). *Warum:* Das Modell allein kann keine Dateien editieren oder Tests laufen lassen — der Harness übersetzt Tool-Calls in echte Aktionen. Verschiedene Harnesses am selben Modell sind selbst ein Messergebnis.

**3. Sandbox** — Benchmark-Agenten laufen in rootless podman (`agent-bench`-Image) und sehen ausschließlich `/work`, eine Wegwerf-Kopie des Task-Workspace. Netz nur über pasta; der Host-Loopback ist als `169.254.1.2` gemappt, damit der Agent den Modell-Server erreicht. *Warum:* Ein unbeaufsichtigter Agent, der `rm`, `git push` oder `curl` kann, gehört nicht in ein echtes Home-Verzeichnis. Details: [Sicherheitsmodell](sicherheitsmodell.md).

**4. Grader** — nach Container-Ende läuft `bench/tasks/<task>/grade.sh` **auf dem Host** und entscheidet Rot/Grün objektiv (Tests, versteckte Grading-Tests, Manipulationscheck). Ergebnis: eine JSON-Zeile in `bench/results.jsonl`. *Warum host-seitig:* Der Agent kann sein eigenes Grading weder sehen noch manipulieren.

**5. Dashboard** — `dashboard/start.sh` → `http://127.0.0.1:8100`. Reine Python-Stdlib, liest alle Ergebnisdateien **strikt read-only**: Suite-Matrix, Polyglot-Grid, Perf-Charts, UX-Findings, dieses Wiki. Einziger Schreibort: `dashboard/runs/` (Logs der über den Läufe-Tab gestarteten Benchmarks, nur Allowlist-Skripte). *Warum:* Die Rohdaten bleiben die Wahrheit; das Dashboard ist nur eine Sicht darauf.

## Schnellstart

```bash
~/ai-lab/serve.sh qwen38 vulkan          # Modell-Server
~/ai-lab/dashboard/start.sh              # Dashboard auf :8100
~/ai-lab/bench/run-task.sh qwen38-vulkan agora-A1-gate   # ein Suite-Task
```

Vertiefung: [Server & Flags](server-und-flags.md) · [Benchmarks verstehen](benchmarks-verstehen.md) · [Ergebnisse lesen](ergebnisse-lesen.md)

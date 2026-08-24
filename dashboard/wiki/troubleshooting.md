# Troubleshooting

**Frage dieser Seite: Ein Lauf ist komisch — welcher der bekannten Fälle ist es diesmal?**

Alle Fälle hier sind real aufgetreten (Benchmark-Wochenende 23./24.08.2026).

## Antwort kommt, aber `content` ist leer

**Fall:** qwen38 im UX-DOM-Test — der Lauf endete „sauber", aber ohne Text: das Thinking fraß die kompletten 6000 `max_tokens`, für die eigentliche Antwort blieb nichts übrig.
**Fix:** Budget rauf — `ux-dom-test.py` liest die Umgebungsvariable:

```bash
MAX_TOKENS=12000 python3 ~/ai-lab/bench/webapp/ux-dom-test.py qwen38
```

Alternativ Thinking abschalten (`enable_thinking: false` via `chat_template_kwargs`). Merksatz: leerer content = Budget-Problem, kein Modell-Urteil.

## Vision-Lauf leer oder mittendrin abgeschnitten

**Fall:** muse UX-Vision Runde 2 — 5 Screenshots sprengen das 16k-Kontextfenster der Vision-Zweige; qwen38-r2 wurde mittendrin abgeschnitten (hatte den Kernbefund, das Flackern, aber schon geliefert).
**Fix:** Shots auf 800 px verkleinern **und** Server mit mehr Kontext starten:

```bash
~/ai-lab/serve.sh muse-vision vulkan -c 24576
```

## Gemini-Suite-Task stirbt nach Sekunden

**Fall:** `oc-gemini37f`-Zeilen mit `seconds` 1,4–11,8, `exit:1`, leerem `changed` — der OpenAI-kompatible Gemini-Endpoint (`…/v1beta/openai`) scheitert bei agentischen Tool-Call-Ketten am `thought_signature`-Handling [genauer Fehlertext unverifiziert — die fehlgeschlagenen Logs wurden von den Wiederholungen überschrieben].
**Fix:** nativen Google-Provider in OpenCode benutzen statt des OpenAI-kompatiblen; der braucht den Key als `GOOGLE_GENERATIVE_AI_API_KEY` — `run-task-api.sh` reicht diese Variable bereits in den Container durch:

```bash
export GOOGLE_GENERATIVE_AI_API_KEY="$GEMINI_API_KEY"
```

Beleg, dass es danach lief: die erfolgreichen Transkripte tragen `google.thoughtSignature`-Metadaten, und alle 5 Tasks sind PASS (88–343 s).

## codernext: VRAM-Übersubscription

**Fall/Mechanik:** Beim 80B-MoE entscheidet `--n-cpu-moe`, wie viele Experten-Schichten im RAM bleiben. Zu **kleiner** Wert = zu viel auf der GPU = VRAM-Übersubscription; der Treiber pagt, der Durchsatz kollabiert bzw. der Load schlägt fehl [konkreter Kollaps-Vorfall unverifiziert — kein Log erhalten]. Der kalibrierte Wert im `serve.sh`-Zweig ist **26** (hält < ~19 GB VRAM, ~26 tok/s laut Server-Log).
**Fix:** bei Symptomen (extrem zäh, OOM beim Laden) `--n-cpu-moe` erhöhen, nicht senken.

## Polyglot: „exhausted context windows" (27B)

**Fall:** Der qwen38-Polyglot-Lauf hatte bei 32k Kontext **17 von 73 Übungen** mit erschöpftem Kontextfenster (Endergebnis trotzdem 72,6 % pass@2 — ohne die Erschöpfungen wäre mehr drin gewesen). Treiber sind u. a. Go-Panic-Traces mit 1000-Zeilen-Goroutine-Dumps im Feedback.
**Fix:** Server für Polyglot mit großem Kontext starten:

```bash
~/ai-lab/serve.sh qwen38 vulkan -c 131072
```

(Geplant war, den qwen38-Default auf 128k umzustellen — Stand heute steht in `serve.sh` noch 32k.)

## Leerer Output vom Server: „Output tokens: ~0 of 0"

**Fall:** Polyglot `robot-simulator` — beide Versuche ohne jeden Output, `num_error_outputs=2`. Inferenz-/Serverfehler, das Modell hat nie Code gesehen.
**Fix:** Retry; `max_tokens`/Server-Log prüfen. Nicht als Modell-Fail werten.

## Schnelldiagnose-Tabelle

| Symptom in results.jsonl | Bedeutung |
|---|---|
| `exit:124` | 1200-s-Timeout (SIGTERM) hat den Container beendet; gegradet wird der erreichte Stand |
| `exit:137` | SIGKILL nach kill-after — z. B. qwen38/U2: 1230 s, trotzdem `PASS pass=52` |
| `FAIL` + `changed` leer + Fehler in `stderr.log` | Infrastrukturproblem, nicht Modellschwäche — Ursache beheben, Task wiederholen (letzte Zeile zählt) |
| `FAIL` + echter Diff | Messergebnis. Stehen lassen. |

**Immer zuerst:** `bench/runs/<label>/<task>/stderr.log` lesen. — Und wenn ein frisch geladenes GGUF Ladefehler oder stillen Unsinn produziert: erst `sudo pacman -Syu` (neue Architekturen brauchen frisches llama.cpp), dann neu testen.

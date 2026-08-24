# Modelle im Lab

**Frage dieser Seite: Welches Modell nehme ich wofür — mit den echten Zahlen dieses Wochenendes?**

Suite = 5 Repo-Tasks (letzte Zeile je Modell+Task); Polyglot = aider python+go, 73 Übungen, pass@2. Zeiten: Summe der 5 Suite-Tasks.

## Lokal (alle auf der RX 7900 XT, Vulkan)

### Qwen3.8-27B (`qwen38`) — der Qualitäts-Pick
- IQ4_XS, 13,3 GiB, dicht, Vision-Variante vorhanden. 32k Kontext, komplett in VRAM.
- **Suite 5/5** (Summe ~37 min; U2 brauchte 1230 s und lief ins Timeout-SIGKILL, bestand aber mit 52/52).
- **Polyglot 72,6 %** pass@2 (53/73; pass@1 43,8 %) — trotz 17 Kontextfenster-Erschöpfungen bei 32k.
- Forensik: null Fails, die **Referenz** — hat denytools genau so gebaut, wie man es bauen sollte (ein alias-bewusster Pfad, Throw an der richtigen Stelle). Grounding-Test 3/3. Schwäche: langsam; UX-DOM-Lauf brauchte wegen Thinking-Budget einen zweiten Anlauf.

### Qwen3.6-35B-A3B (`qwen36moe`) — der Speed-Pick
- MoE, 3B aktiv, Q3_K_XL, 15,7 GiB. ~122 tok/s Generierung (Vulkan, Kontext 0), 109 tok/s noch bei 32k Tiefe.
- **Suite 4/5** (Summe ~14 min; A1 in 23,5 s, A2 in 22,6 s — mit Abstand am schnellsten).
- **Polyglot 63,0 %** pass@2 (46/73; pass@1 37,0 %).
- Forensik: der eine Fail (U2) war reiner Regelverstoß — Feature komplett korrekt, aber Tests an eine bestehende Testdatei angehängt; mit zurückgesetztem Testverzeichnis 52/52. „Effektiv" 5/5. Mit Regeln 1+5 aus `failure-analysis.md` gut steuerbar.

### Muse-Glimmer-30B (`muse`) — der Allrounder
- Q4_K_XL, 15 GiB, Vision-Variante vorhanden. ~38 tok/s Generierung, hält Tempo bei Tiefe (36,5 @32k).
- **Suite 5/5** (Summe ~19 min) — null Fails, keine Korrekturen nötig.
- **Polyglot: kein vollständiger Lauf** — zwei Anläufe am 24.08. brachen nach 2 bzw. 11 Übungen ab; keine belastbare Zahl.
- Besonderheit ROCm: einziges Modell, bei dem ROCm das Prompt-Processing klar gewinnt (906 vs. 756 tok/s), Vulkan aber die Generierung.

### Qwen3-Coder-Next 80B-A3B (`codernext`) — das große Coding-Modell
- MoE, Q3_K_XL, 33,8 GiB — läuft nur mit `--n-cpu-moe 26` (Experten im RAM), ~26 tok/s.
- **Suite 3/5** (Summe ~11 min — schnell, weil es wenig verifiziert).
- Polyglot: nicht gefahren. Perf-Matrix: fehlt.
- Forensik: zwei verschiedene Schwächen — (a) delegiert komplett an Subagenten und verliert dabei Constraints (A3: Feature korrekt, Testfile-Regel ging an der Delegationsgrenze verloren); (b) führt eigenen neuen Code nicht aus und behauptet trotzdem Erfolg (U2: fehlender Import + Alias-Lücke, beides Ein-Zeilen-Fixes). „Effektiv" 4/5. **Für unbeaufsichtigte Läufe derzeit die schwächere Wahl gegenüber 27B — wegen Disziplin, nicht Fähigkeit.**

## Cloud-Vergleich (gleiche Suite, gleiche Grader)

| Modell (Harness) | Suite | Zeit gesamt | Polyglot pass@2 |
|---|---|---|---|
| DeepSeek v4-flash (dsh) | **5/5** | ~6 min | Lauf am 24.08. noch nicht abgeschlossen |
| DeepSeek v4-pro (dsh) | **5/5** | ~7 min | Lauf am 24.08. noch nicht abgeschlossen |
| Gemini 3.7 Flash (OpenCode, nativer Provider) | **5/5** | ~16 min | **95,9 %** (70/73; pass@1 87,7 %; 15,7 s/Übung) |

Einordnung: Die Cloud-Modelle sind schneller und bei Polyglot klar vorn (Gemini 95,9 % vs. 72,6 % lokal). Die Suite dagegen bestehen die besten lokalen Modelle (27B, Muse) genauso mit 5/5 — für agentische Repo-Arbeit ist lokal konkurrenzfähig, kostenlos und offline. DOM-Steuerung (info/form auf der Testseite) haben alle sechs getesteten Modelle absolviert — lokal wie Cloud; Bewertung im Apps-&-UX-Tab.

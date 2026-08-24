# Fahrplan (Stand 24.08. ~19:15, Deadline 25.08. ~12:00)

MORGEN-NACHZÜGLER (nach Polyglot-Ende, GPU frei):
- **A4+A5+A6 für die LOKALEN Modelle** (Tobias 24.08. ~23:30: Qwen direkt als
  **qwen38-vision** servieren — Vision kostet nichts, Label bleibt qwen38-vulkan;
  dazu muse, qwen36moe; **codernext FIX als vierter, ZULETZT** (Tobias 24.08. ~23:45:
  **ALLES Ungetestete**: Suite A4/A5/A6, voller Polyglot, DOM-Tests (dom-agent info+form),
  UX-DOM-Review, llama-bench-Tiefenmatrix (fehlt komplett — Dashboard sagt „keine Messung").
  Vision entfällt (kein mmproj für Coder-Next). Begründung: Suite-Speed war fast 35B-Klasse
  [3B aktiv]. Polyglot-80B ≈ 4–5 h GPU → läuft über die 12-Uhr-Grenze hinaus fertig; Skripte
  brauchen keine Session, Ergebnis erscheint automatisch im Dashboard):
  je Modell `serve.sh <m> vulkan`, dann `bench/run-task.sh <label> agora-A4-feed`,
  `... agora-A5-batcher-scratch 1800`, `... agora-A6-scorer-scratch 1800`.
  Cloud-Referenz: A4 3/3 PASS · A5 3/3 FAIL (einstimmig an der sortiert/sortierbar-Ambiguität,
  je 5/6 Properties) · A6 3/3 PASS. A5-Regrade wartet auf Tobias' Mobil-Entscheidung.
- qwen38 UX-DOM wiederholen: content war LEER (Thinking fraß die 6000 max_tokens) →
  max_tokens 12000 ODER enable_thinking:false via chat_template_kwargs in ux-dom-test.py.
- muse UX-Vision Runde 2 wiederholen: content leer (5 Bilder sprengen 16k-ctx) →
  Shots auf 800px verkleinern + muse-vision mit -c 24576 starten. qwen38-r2 dito (war
  mittendrin abgeschnitten), hat aber das FLACKERN aus den 2 Frames erkannt (Kernbefund steht!).
- UX-Wertungstabelle (Runde 1+2, alle Modelle) in den Bericht.

NEU (Tobias-Wünsche vom Abend): Muse-Polyglot + API-Tests laufen PARALLEL (GPU vs. Cloud).
**Dashboard-App v1** wird heute Nacht gebaut (Agent läuft): lokale Web-App ~/ai-lab/dashboard/
auf :8100 — Ergebnisse visuell (Suite-Matrix, Polyglot-Grid, Perf-Charts, UX-Findings), Läufe-Tab
zum Selbst-Testen künftiger Modelle, Wiki-Tab (alle Docs) + Guide für neue Benchmarks
(unser tasks/-Format = das Plugin-Format). Am Ende: **Doku-Update/-Check** über alles.

# Alter Fahrplan 24.08. (nach der Polyglot-Nacht)

1. **Polyglot-Kette abschließen**: 35B ✅ (63%) → Muse-Suite ✅ (5/5!) → 27B (läuft, ~78%) →
   **llama-bench für Muse nachholen** (Vulkan+ROCm, d 0/10k/32k — fehlt im Bericht!) → Muse-Polyglot → 80B (getrimmt).
2. **Bericht-Update**: Polyglot-Spalte für alle Modelle + **Muse-Suite-Zeile (5/5!)** + Speed-first-Empfehlung
   (Tobias' Gewichtung: Agency/Tempo > Regeltreue) + **neue Sektion „Was nicht geklappt hat" pro Modell**:
   Fehlerursache, Schweregrad, steuerbar-oder-Fähigkeitsgrenze (Forensik-Workflow läuft in der Nacht,
   Ergebnis in ~/ai-lab/bench/failure-analysis.md).
2b. **Komplexitäts-Test „Personalisierungsalgorithmus"** (Tobias' Wunsch): den Feed-/Personalisierungs-
   Algorithmus in agora-backend (internal/feed) von den lokalen Modellen reviewen lassen (Aufgabe:
   Architektur erklären, Schwachstellen finden, ggf. eine gezielte Verbesserung mit Test). Grading
   qualitativ gegen eine Referenz-Review von mir. Tobias hat den Algorithmus selbst noch nicht reviewt.
3. **Browser/UX-Block — ALLE Modelle, nicht nur die Vision-fähigen (Tobias: „muss nicht native
   Vision sein, muss nur funktionieren")**:
   a) **DOM-Pfad Browser (alle 4 + später API-Modelle):** Playwright-artige Navigation über
      Accessibility-Snapshots als Text (Elemente + Referenzen) — Aufgaben wie „navigiere zu X,
      fülle Formular, finde Info Y", objektiv verifizierbar. Harness: Playwright-MCP an OpenCode
      oder kleiner Eigenbau-Loop. Sicheres Ziel: lokale Testseite / harmlose öffentliche Seite.
   a2) **Desktop-Apps kontrollieren (alle Modelle, Tobias' Wunsch):** AT-SPI-Accessibility-Baum
      als Text-Sicht auf GTK/Qt-Apps + xdotool zum Klicken/Tippen (Referenz: agent-sh/computer-use-linux).
      Aufgaben im Stil „öffne Mousepad, tippe Text, speichere als Datei" (Verifikation: Datei existiert),
      idealerweise im Xvfb-Nebendesktop statt in Tobias' Live-Session.
   b) **Vision-Pfad (nur Qwen3.8 + Muse, mmproj vorhanden):** Screenshot → Zoom → Klick-Loop
      (xdotool), aufbauend auf dem 3/3-Grounding-Ergebnis.
   c) **„UX-Probleme erkennen":** Screenshot einer UI rein → Befundliste raus (Vision-Modelle);
      für Text-Modelle Vergleichsvariante mit DOM/HTML als Input.
4. **API-Vergleich — Tobias' finaler Plan (24.08., ~15:30):**
   a) ZUERST: DeepSeek mit **DeepSeek-eigenem Harness** (recherchieren, was deren offizielles
      Agent-CLI Aug 2026 ist!), sowohl **v4-flash als auch v4-pro**, gegen die 5-Task-Suite.
   b) DANACH als Vergleichswerte via **OpenCode**: DeepSeek flash, DeepSeek pro, **Gemini 3.7 Flash**
      — jeweils **Repo-Suite + Polyglot-Subset**. **Gemini 3.1 Pro ist GESTRICHEN.**
   c) Kosten neu geschätzt inkl. Polyglot: Suite-Läufe je 0,08–1 €; Polyglot je ~0,40 € (flash) /
      ~1,20 € (pro) / ~2,10 € (Gemini Flash). Gesamt alle Läufe: ~6–9 €. Off-Peak (Nachmittag/Abend) nutzen.
   Danach liest Tobias alles und entscheidet, was er sich einrichtet.
4b. **Forensik-Konsequenzen ins Setup** (Ergebnis liegt in bench/failure-analysis.md): die 5 AGENTS.md-Regeln
   in die DAILY-Config `~/.config/opencode/AGENTS.md` schreiben (NICHT in bench/opencode-config — die
   Benchmark-Läufe müssen untereinander vergleichbar bleiben, auch die API-Vergleiche morgen).
   Im Bericht die „effektive Fähigkeits-Wertung" ergänzen: 35B wäre 5/5, 80B 4/5 ohne die Regelverstöße
   (Re-Grading mit zurückgesetzten Testdateien: beide PASS). serve.sh qwen38 auf 128k Kontext umstellen.
5. **Entscheidung** mit Tobias: Daily-Driver-Modell + was bleibt installiert.
5. **AUFRÄUMEN** (explizit gewünscht — „keine verwirrenden Branches"):
   - Klarstellen/prüfen: ~/Projects-Repos wurden nie berührt, dort existieren KEINE Bench-Branches.
   - `~/ai-lab/bench/workspaces` + `runs` löschen; vorher die brauchbare U2-Lösung
     (denyTools, 52/52) als Patch sichern: `git -C runs/qwen38-vulkan/aiux-U2-denytools/ws diff bench/local-agent > ~/ai-lab/denytools-qwen38.patch`.
   - `smoke-ws`, Screenshot-Dateien, `u2-grade-dev`-Scratch weg; aider-Ergebnisse behalten.
   - Podman: `agent-bench` + `aider-bench` Images behalten oder löschen → Tobias fragen (je ~1-2 GB).
5b. OPTIONAL (nach Deadline, Tobias-Idee 24.08. abends): **Gemini-Abo vs. API vergleichen** —
   Googles Gemini CLI als drittes Harness andocken (Muster: run-task-dsh.sh; npm @google/gemini-cli
   in Containervariante, Abo-Auth = einmaliger OAuth auf dem Host, ~/.gemini mounten). Fragestellung
   ist Kosten/Quotas, nicht Qualität (gleiches Modell). Suite als Vergleich + auf Tobias' Wunsch
   (24.08. ~0:15) auch ein **Polyglot-Lauf via Gemini CLI** (Runner nach dem Muster von
   bench/polyglot-oc bauen; Kernfrage: übersteht die Abo-Quota 73 Übungen + wie ist die Wandzeit
   vs. API?). Fakt zur Bindung: Abo-Kontingent läuft NUR über Googles eigene Tools (OAuth),
   Fremd-Harnesses brauchen API-Key — Muster wie bei Anthropic.
   Generell bestätigt: OpenCode + API-Key ist der Standard-Harness-Weg.
6. **Setup auf GitHub sichern** (Tobias' Wunsch): ~/ai-lab als Git-Repo initialisieren, .gitignore
   (models/, logs/, bench/runs/, bench/workspaces/, bench/aider/aider*, smoke-ws, Screenshots),
   privates Repo unter LoweloDev anlegen (`gh repo create ai-lab --private`), pushen. Rein: Skripte,
   Tasks+Grader, opencode-config, README, TODO, results.jsonl, failure-analysis.md.

# Nachtrag 24.08. ~23:15 (Tobias): Kandidaten-Runde 100-130B-MoE
- Tobias will "wenn nachher noch Zeit ist" Qwen3.5-122B-A10B (bzw. was er damit meint) auf dem
  Setup testen (Experten-Offload wie beim 80B) + 1-2 aehnlich grosse MoE-Modelle ANDERER Marken
  als Vergleich. Recherche-Agent laeuft; Ergebnis = Kandidatentabelle + Download-URLs.
  Rechnung: 122B-A10B Q3_K_XL ~52 GiB (> RAM+VRAM ~50 GiB -> NVMe-Streaming) oder Q2 ~40 GiB;
  tg-Erwartung ~8-14 t/s (3,3x aktive Parameter vs. 80B-A3B mit 28-41 t/s); Prefill wird der
  Engpass bei agentischer Arbeit. Ehrlich: Experiment, kein Daily-Driver ohne 64 GiB RAM.
- Reihenfolge: NACH den Qwen-Retries (GPU frei), Download (40-52 GiB je Modell) vorher.
  serve.sh-Case anlegen (--n-cpu-moe hoeher als 26), llama-bench-Matrix, dann Suite A1-A6.

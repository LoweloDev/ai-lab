# Fahrplan (Stand 24.08. ~19:15, Deadline 25.08. ~12:00)

MORGEN-NACHZÜGLER (5-10 min, nach Polyglot-Ende, GPU frei):
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
6. **Setup auf GitHub sichern** (Tobias' Wunsch): ~/ai-lab als Git-Repo initialisieren, .gitignore
   (models/, logs/, bench/runs/, bench/workspaces/, bench/aider/aider*, smoke-ws, Screenshots),
   privates Repo unter LoweloDev anlegen (`gh repo create ai-lab --private`), pushen. Rein: Skripte,
   Tasks+Grader, opencode-config, README, TODO, results.jsonl, failure-analysis.md.

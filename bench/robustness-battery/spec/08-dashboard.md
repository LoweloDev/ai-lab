# 08 — Dashboard-Integration (alle 8 Tasks)

Lies /home/lowelodev/ai-lab/dashboard/labdata.py (wie /api/suite entsteht: Entries, Superseding, Labels), /home/lowelodev/ai-lab/dashboard/server.py (Routen; Job-Typ-Liste bei 'suite','suite-api',
'dom','uxdom','polyglot'), /home/lowelodev/ai-lab/dashboard/labjobs.py (Job-Ausfuehrung, GPU-Lock), /home/lowelodev/ai-lab/dashboard/static/app.js (renderSuite inkl. Gesamtzeit-Balkengraph 'bars'/'bar-row',
viewHead, chip, fmtSec, esc), /home/lowelodev/ai-lab/dashboard/static/style.css (Design-Tokens, Type-Scale), ein Wiki-Beispiel unter /home/lowelodev/ai-lab/dashboard/wiki/.
Hinweis: Im Container gibt es KEIN python3 — Python-Aenderungen sorgfaeltig und minimal, syntaktisch konservativ (keine neuen Abhaengigkeiten,
nur stdlib), JS mit `node --check` pruefen. Der Host-Pruefer kompiliert und startet den Server danach.
1. labdata.py: /home/lowelodev/ai-lab/bench/robustness-battery/results.json (Schema 2) lesen (fehlt => leere Struktur). Staleness je (task,label): Fingerprint des ws nach derselben Regel
   wie der Runner (sha256 ueber sortierte Pfad+Inhalt der Nicht-Test-Quellen unter cwd; 30-s-Cache). /api/suite: je Entry ein Feld
   robust = {real_pass, real_total, path_pass, path_total, buildable, failed, stale} oder null (keine Batterie fuer den Task).
   Neuer Endpunkt /api/robustness: {batteries, results, scores: {<label>: {real_score, real_pass, real_total, path_pass, path_total,
   n_tasks, n_missing, per_task: {<task>: {…}}}}, stale: [[task,label],…]}; real_score nach Konvention.
2. app.js, Suite-Ansicht: (a) jede Zelle mit robust zeigt eine zweite Mini-Zeile 'R 11/12 · P 5/5' (klein, muted; 'n. baubar' bei buildable:false;
   ⟳ bei stale); Zellen-Detail listet die gerissenen Tests. (b) Neue Spalte GANZ RECHTS 'Robustheit' je Label: real_score in Prozent +
   Mini-Balken + 'n/8'. (c) Unter dem Gesamtzeit-Graphen ein Balkengraph 'Robustheit ueber alle Suite-Tasks (Zusatz-Metrik, aendert keine Urteile)'
   je Label, sortiert nach real_score, Balken = real_score, Pill = Path-Quote, Klick klappt per_task auf; Legende (ein Satz Real/Path) + Link
   auf das Wiki 'robustheit'. (d) Laeufe-Tab: Job-Typ 'robustheit' ("Robustheit neu berechnen", CPU, KEIN GPU-Lock) fuehrt /home/lowelodev/ai-lab/bench/robustness-battery/run-all.sh aus,
   Ausgabe im Job-Log (server.py + labjobs.py analog zu bestehenden Job-Typen). Nur vorhandene Design-Tokens, beide Themes, Type-Scale, tabular-nums.
3. Wiki /home/lowelodev/ai-lab/dashboard/wiki/robustheit.md (deutsch): Was/warum Zusatz-Metrik, die zwei Stufen, Gesamt-Score-Formel, wie lesen, wie neu berechnen, Grenzen,
   Verweise auf a5-results.md/a6-results.md/audit-report.md. In den Wiki-Index einhaengen (nachsehen, wie andere Artikel registriert sind).
4. `node --check /home/lowelodev/ai-lab/dashboard/static/app.js`. Liste in done-08.txt alle geaenderten Dateien und was der Host-Pruefer starten/pruefen muss.

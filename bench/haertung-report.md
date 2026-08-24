# Grader-Härtung — Integrationsbericht und Anleitung

Stand: 25.08.2026, ~00:45. Grundlage: `audit-report.md` (Befunde F1–F14, Patch-Pakete P1–P8, Referenz-Skelett 4.1,
Politik 4.4) und die Validierung `audit-scratch/haertung/validierung.md`. Pfade relativ zu
`/home/lowelodev/ai-lab/bench/`. Es wurde nichts an Live-Dateien geändert: `tasks/*/grade.sh`, `grade_test.go`,
`run-task*.sh`, `results.jsonl`, `workspaces/`, `runs/` sind unberührt. Die Anwendung erledigt `apply-haertung.sh`
**nach** den Ketten (`haertung-waechter.sh` ruft es nach `A5A6-RETRY KOMPLETT`, sofern `.haertung-freigabe` liegt).

---

## 1. Was geändert wird (Paket → Datei)

| Paket | Befunde | Staging-Datei (liegt bereits) | wird installiert als | Tasks |
|---|---|---|---|---|
| P1 Go-Grader-Skelett | F1, F2, F3, F6, F10, F11 | `tasks/<t>/grade.v2.sh` | `tasks/<t>/grade.sh` | A1, A3, A4, A5, A6 |
| P1 (A3-Heredoc ausgelagert) | F3 | `tasks/agora-A3-hls/grade_test.v2.go` | `tasks/agora-A3-hls/grade_test.go` (neu) | A3 |
| P2 Node-Grader | F2, F4, F7, F10 | `tasks/<t>/grade.v2.sh` | `tasks/<t>/grade.sh` | A2, U1, U2 |
| P4 A6-Properties (Permutation × ID-Zuordnung) | F5 | `tasks/agora-A6-scorer-scratch/grade_test.v2.go` | `tasks/agora-A6-scorer-scratch/grade_test.go` | A6 |
| P5 A5-Properties | F8, F9, F12 | `grade_test.v2.go` (streng: (c) permutiert, (b) entschärft, (e2)) **oder** `grade_test.v2-vertrag.go` (Vertragslesart von (c)) | `tasks/agora-A5-batcher-scratch/grade_test.go` | A5 |
| P6 U2-Tooltabelle + Laufzeit-Sweep | F13 | `tasks/aiux-U2-denytools/grade.v2.test.js` | bleibt unter diesem Namen; vom installierten `grade.sh` referenziert (`grade.test.js` bleibt unverändert) | U2 |
| P8 Defense-in-depth-Grep | Restrisiko F1 | in jedem `grade.v2.sh` enthalten | nur `WARN`-Zeilen vor der Urteilszeile, nie ein Urteil | alle |
| P3 Runner (Baseline-SHA außerhalb `/work`, `timeout 900` um den Grade-Schritt) | F2, F11 | — | **nicht Teil dieser Anwendung** (Runner sind read-only, solange Ketten laufen); die v2-Grader ignorieren `ws/.bench-baseline` ohnehin und tragen eigene Timeouts | — |
| P7 Brief A2 (löschbare TS-Syntax) | F14 | — | nicht angewendet (nur Brief-Hinweis; false-fail, vom Modell selbst erkennbar) | A2 |

Gemeinsame Eigenschaften aller acht v2-Grader (siehe Kopfkommentare der Dateien): `set -euo pipefail`, absolute
Pfade, Bewertung in einer Wegwerf-Kopie (`mktemp -d`, `trap`-Cleanup) — **der Workspace wird nie verändert**;
Tamper-Guard byteweise (`cmp`) gegen `workspaces/<task>` statt gegen modell-beschreibbaren Git-Zustand;
versteckte Tests verifiziert einkopiert (Kollision/Symlink = FAIL); Go: `go test -json` unter `timeout`, PASS nur bei
Paket-pass **und** Pass-Ereignis je erwartetem Test; Node: verankerter `^ℹ pass N$`/`tail -1`-Parser plus Exit-Code
plus exakte Pass-Zahl. Modell-eigene Testdateien bleiben im Lauf (Brief-Semantik); Kaperung ist über die
Pass-Ereignisse neutralisiert. Aufruf-Vertrag unverändert: `bash grade.sh <ws>`, letzte stdout-Zeile `PASS…|FAIL…`,
keine Runner-Änderung nötig.

**Installations-Detail:** `apply-haertung.sh` kopiert `grade.v2.sh` nach `grade.sh` und ersetzt dabei außerhalb von
Kommentaren `grade_test.v2.go` durch `grade_test.go` (A3: fester Pfad; A5/A6: `HIDDEN=${GRADE_HIDDEN:-…}`), damit
das Installat die Live-Datei `grade_test.go` liest. Zeile 2 des Installats trägt Zeitstempel, Lesart und Herkunft.
Die v2-Dateien bleiben liegen (Staging = Referenz; A5 hält beide Lesarten vor).

---

## 2. Validierung (Validator-Lauf 25.08., `audit-scratch/haertung/validierung.md`)

### 2.1 Vollvergleich alt vs. v2 auf allen 98 Abgaben — 0 Flips

| Task | n | gleiche Klasse | Flips |
|---|---|---|---|
| agora-A1-gate | 13 | 13 | 0 |
| agora-A2-jsonld | 12 | 12 | 0 |
| agora-A3-hls | 12 | 12 | 0 |
| agora-A4-feed | 12 | 12 | 0 |
| agora-A5-batcher-scratch | 12 | 12 | 0 (strenge Lesart) |
| agora-A6-scorer-scratch | 12 | 12 | 0 |
| aiux-U1-paging | 13 | 13 | 0 |
| aiux-U2-denytools | 12 | 12 | 0 |
| **Summe** | **98** | **98** | **0** |

Workspaces nach jedem Lauf byteweise unverändert (`git status` vor/nach identisch, Voll-Baum-SHA in 10er-Stichprobe
IDENT), keine Grader-Rückstände, keine verwaisten `/tmp/tmp.*`. Nur die FAIL-Begründungen werden präziser
(z. B. `missing=[…]`, Dateiname bei `test-file-modified`); **U2-PASS-Strings ändern sich von `pass=52` auf
`pass=58`** (P6 fügt 6 Tests hinzu) — gleiche Klasse, wird beim Regrade *nicht* in `results.jsonl` übernommen
(Politik 4.4 Nr. 3/5: nur Klassenwechsel), sondern nur im Regrade-Log vermerkt.

### 2.2 Exploit-Batterie — 36 Angriffe geschlossen, 0 offen

| Angriff | Task | Befund | alt | v2 | Status |
|---|---|---|---|---|---|
| a5-init (init os.Exit, Nicht-Testdatei) | A5 | F1 | PASS | FAIL | geschlossen |
| a5-tmain (TestMain os.Exit) | A5 | F1 | PASS | FAIL | geschlossen |
| a5-flag (flag.Set test.skip) | A5 | F1 | PASS | FAIL | geschlossen |
| a5-sys (syscall.Exit) | A5 | F1 | PASS | FAIL | geschlossen |
| a5-init2 (a5aud-init) | A5 | F1 | PASS | FAIL | geschlossen |
| a5-chmod-F3 (chmod 555 feed) | A5 | F3 | PASS | FAIL | geschlossen |
| a5-git-rm / -assume / -commit / -gc (4 Tamper-Varianten) | A5 | F2 | PASS | FAIL | geschlossen (×4) |
| a5-livecap-F9 (per-page cap) | A5 | F9 | PASS | FAIL | geschlossen |
| a6-idsort-F5 (e1), a6-revsort-F5 (e1b), a6-idsort-poc2, a6-revsort-poc2 | A6 | F5 | PASS | FAIL | geschlossen (×4) |
| a6-tmain (e2a), a6-init (e2b), a6-poc2-tmain | A6 | F1 | PASS | FAIL | geschlossen (×3) |
| a6-git-e3a…e3e (5 Tamper-Varianten) | A6 | F2 | PASS | FAIL | geschlossen (×5) |
| a6-chmod-F3 (chmod 555 feed) | A6 | F3 | PASS | FAIL | geschlossen |
| a1-hijack (untracked TestMain), a1-untracked (zz_grade_hijack) | A1 | F1 | PASS | FAIL | geschlossen (×2) |
| a1-nobase, a1-commit-t, a1-mod-repoint | A1 | F2 | PASS | FAIL | geschlossen (×3) |
| a3-hijack (TestMain) | A3 | F1 | PASS | FAIL | geschlossen |
| a3-chmod-F3 (chmod 555 livehls) | A3 | F3 | PASS | FAIL | geschlossen |
| a2-inject-F4 (console.log ℹ pass) | A2 | F4 | PASS | FAIL | geschlossen |
| a2-notwired-F7 (JSON.stringify-Bypass) | A2 | F7 | PASS | FAIL | geschlossen |
| u1-inject-F4 (console.log ℹ pass) | U1 | F4 | PASS | FAIL | geschlossen |
| u2-cheat-F13 (nur 2 Tools gesperrt) | U2 | F13 | PASS | FAIL | geschlossen |
| u2-tamper-F2 (commit weaken + repoint) | U2 | F2 | PASS | FAIL | geschlossen |

Kontrollen (korrekt, kein Flip): a5-control, a5-git-ctrl, a5-symlink, a6-git-e3ctrl, a6-stub-e0, a6-poc2-ctrl,
a3-base-ctrl, a2-base-ctrl, a2-enum-F14, u2-chmod-F3 — alle FAIL/FAIL; positive Kontrolle a5-orig-ref
(legitimer Original-Batcher) PASS/PASS.

---

## 3. Erwartete Flips beim Regrade

| A5-Lesart | erwartete Flips | Grundlage |
|---|---|---|
| **streng** (Default; `.a5-lesart` fehlt oder `streng`) | **0** über alle 98 (+ Nachzügler-Retries) | Vollvergleich 2.1; Vorabprüfung audit-report 4.2 (keine Hijack-Muster, Kept-Tests identisch bis auf 2 bereits-FAIL-Fälle) |
| **vertrag** (`.a5-lesart` = `vertrag` oder `GRADE_A5=vertrag`) | **genau 8, alle A5 FAIL → PASS:** agy-37flash, cc-opus48, cc-opus5, dsh-v4-flash, dsh-v4-pro, oc-gemini37f, oc-v4-flash, oc-v4-pro; codernext, muse, qwen36moe, qwen38 bleiben FAIL | audit-report F8-Tabelle (`synth/a5-double-score.sh`), P5 |

Jeder Flip außerhalb dieser Menge ist laut Audit 4.2 ein echter Fund (ein Modell hätte eine Lücke benutzt) und
führt zum **Abbruch ohne Änderung** — auch ein Nachzügler-Retry (qwen36moe/qwen38 A5), der unter der
Vertragslesart PASS würde, zählt als unerwartet und muss einzeln bewertet werden, bevor erneut angewendet wird.

**Trockenlauf 25.08. 00:31 (`audit-scratch/haertung/apply-dry-run.log`, Lesart streng):** siehe Abschnitt 5.

---

## 4. Anleitung

### 4.1 Wann

Erst nach `NACHTKETTE-2 KOMPLETT` **und** `A5A6-RETRY KOMPLETT` (die Kandidaten-Kette wartet ihrerseits auf den
Marker `.grader-haertung-done`). `apply-haertung.sh` verweigert die Anwendung (rc 2, **kein** Marker), solange ein
`run-task*.sh`-, `nachzuegler-a5a6-retry.sh`- oder `nachtkette*.sh`-Prozess läuft; ein Lock
(`.apply-haertung.lock`) verhindert Doppelstarts.

### 4.2 A5-Lesart festlegen (Entscheidung 4.3 des Audits)

```bash
echo vertrag > /home/lowelodev/ai-lab/bench/.a5-lesart     # Option A (empfohlen): Vertragslesart, 8 Flips
# oder: Datei weglassen / 'streng' hineinschreiben          # Option B: (c) streng, 0 Flips
```
`GRADE_A5=vertrag|streng` in der Umgebung überstimmt die Datei. Jeder andere Inhalt bricht mit rc 3 ab.

### 4.3 Anwenden

```bash
cd /home/lowelodev/ai-lab/bench
bash apply-haertung.sh --dry-run          # nur Vorprüfung + Bericht, read-only, kein Marker
touch .haertung-freigabe                  # dann übernimmt haertung-waechter.sh nach den Retries
# oder von Hand, wenn die Ketten durch sind:
bash apply-haertung.sh
cat .grader-haertung-done                 # 'OK <stamp> flips=<n> lesart=<…>' oder 'ABGEBROCHEN …'
```
Ablauf im Skript: (1) Vorprüfung aller letzten `results.jsonl`-Einträge gegen `grade.v2.sh` plus Freilos-Check
(Baseline darf nicht PASS werden) → bei unerwarteten Flips `ABGEBROCHEN`, nichts angewendet; (2) Backup
`results.jsonl.bak-audit-<stamp>`, `grade.sh → grade.v1.sh`, `grade_test.go → grade_test.v1.go`; (3) Swap +
Rauchtest (Baseline FAIL, A5-Referenz `audit-scratch/a5aud-orig` PASS; sonst automatischer Rollback); (4) Regrade
mit dem installierten `grade.sh`, In-Place-Ersatz **nur des `grade`-Felds** bei Klassenwechsel, Log
`audit-scratch/regrade-<stamp>.log`, Nachtrag in `failure-analysis.md`, Einträge `suite:<label>/<task>` in
`dashboard/registry/run-annotations.json` (A5-Vertragslesart: „unter defensiver Lesart FAIL (nur Property c)");
(5) Marker. Zweiter Aufruf: erkennt `grade.v1.sh` und macht nur den Regrade-Vergleich (idempotent). Je Aufruf
liegt ein Bericht unter `audit-scratch/haertung/integrator/{dry,apply}-<stamp>/bericht.md` mit allen
Grader-Logs.

### 4.4 Rollback

```bash
cd /home/lowelodev/ai-lab/bench
for t in tasks/*/; do [ -f "$t/grade.v1.sh" ] && cp -p "$t/grade.v1.sh" "$t/grade.sh" && rm "$t/grade.v1.sh"; done
for t in agora-A5-batcher-scratch agora-A6-scorer-scratch; do cp -p tasks/$t/grade_test.v1.go tasks/$t/grade_test.go; rm tasks/$t/grade_test.v1.go; done
rm -f tasks/agora-A3-hls/grade_test.go                      # A3 hatte vorher keine (Heredoc im alten grade.sh)
cp results.jsonl.bak-audit-<stamp> results.jsonl            # nur, wenn das Regrade Flips eingetragen hat
rm -f .grader-haertung-done                                  # damit ein späterer Lauf die Härtung erneut prüft
```
Nachtrag in `failure-analysis.md` und die `suite:`-Schlüssel in `run-annotations.json` ggf. von Hand entfernen.
Die v2-Staging-Dateien bleiben in jedem Fall liegen.

### 4.5 Danach

Ab dem Regrade sind Grader-Änderungen Suite-Versionen (Politik 4.4 Nr. 6): jede weitere Änderung an
`grade.sh`/`grade_test.go` löst denselben Zyklus aus (Backup, Regrade aller letzten Einträge, In-Place-Korrektur,
Nachtrag). Offen bleiben P3 (Runner: `baseline.sha` außerhalb von `/work`, `timeout 900` um den Grade-Schritt —
erst anfassen, wenn keine Kette läuft) und P7 (Brief A2).

---

## 5. Trockenlauf-Ergebnis (25.08., read-only)

_(wird unten aus `audit-scratch/haertung/apply-dry-run.log` übernommen)_

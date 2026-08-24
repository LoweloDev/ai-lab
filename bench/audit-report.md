# Grader-Audit der Benchmark-Suite — Synthese

Stand: 24./25.08.2026. Gegenstand: die acht Grader unter `tasks/*/grade.sh` (plus `grade_test.go` bei A5/A6,
`grade.test.js` bei U2) und die Runner `run-task*.sh`. Quellen: die Befund-Listen der Task-Auditoren, die
unabhängigen PoC-Läufe `audit-scratch/poc-0` … `poc-3`, und ein Synthese-Nachlauf (24.08. 23:30 – 25.08. 00:30,
`audit-scratch/synth/`), in dem jede Variante noch einmal gegen den **installierten** Grader ausgeführt wurde.
Es wurde nichts unter `tasks/`, `runs/`, `workspaces/` oder in `results.jsonl` verändert; alle Patches sind
Entwürfe. Der Präzedenzfall muse-vulkan/A5 (feste Anordnung in Property (c), bereits mit `gradePermute` gefixt)
wird hier nicht erneut geführt.

---

## 1. Management-Zusammenfassung

Die Grader sind funktional korrekt für ehrliche Abgaben, aber gegen eine Abgabe, die den Grader statt der
Aufgabe adressiert, sind **alle acht** durchlässig: Ein Modell mit Shell im Container kann in jedem Go-Task ein
PASS ohne eine Zeile Fachlogik erzwingen (Test-Binary-Kaperung, schreibgeschütztes Paketverzeichnis), in den
Node-Tasks A2 und U1 mit zwei `console.log`-Zeilen, und den Tamper-Guard aller acht Tasks mit einem Git-Handgriff
oder dem Löschen einer Datei aushebeln. Die zweite Befundklasse sind Löcher in den versteckten Property-Tests
selbst: In A6 besteht ein reiner ID-Sortierer ohne jede Personalisierung sämtliche zehn Properties, in A5 besteht
eine Implementierung ohne Live-Kadenz-Logik, und in A2 reicht `JSON.stringify(jsonLd, null, 0)`, um die
Verdrahtungsprüfung zu umgehen. Beruhigend ist die Rückschau auf die 98 echten Bewertungen: Kein Modell hat eine
dieser Lücken benutzt — keine Kaperungsmuster, alle Paketverzeichnisse beschreibbar, alle 400 behaltenen
Testdateien byteweise mit der Baseline identisch bis auf die zwei bereits als `test-file-modified` gewerteten Fälle —
und die gehärteten Grader liefern für alle vorhandenen Abgaben dieselben Urteile, mit einer Ausnahme, die eine
Entscheidung braucht: A5-Property (c) verlangt defensives Selbst-Sortieren, das der Brief nicht fordert; unter der
Vertragslesart kippen 8 der 12 A5-Urteile von FAIL auf PASS, während muse, codernext und die beiden Qwen-Abgaben
auch dann durchfallen. Empfehlung: nach Abschluss der laufenden Ketten alle Grader auf das gehärtete Skelett heben
(Pass-Ereignisse je Test statt Exit-Code, Referenz außerhalb des Workspaces, verifizierte Kopien, Timeouts),
sämtliche Einträge in place neu bewerten (erwartete Flips: 0 bis auf A5), und die A5-Lesart per Entscheidung
festlegen.

---

## 2. Grader-Befunde nach Schwere

Legende PoC-Status: **bestätigt (PoC-N)** = unabhängiger PoC-Lauf des Orchestrators; **bestätigt (Nachlauf)** =
im Synthese-Nachlauf gegen den installierten Grader reproduziert; **verworfen** = Angriff läuft ins Leere;
**ungeprüft** mit Begründung. Die Orchestrator-PoC-Liste war nach PoC-3 abgeschnitten; alles danach stammt aus dem
Nachlauf. Pfade relativ zu `/home/lowelodev/ai-lab/bench/`.

### Schwere HOCH

#### F1 — Test-Binary-Kaperung: Grader werten nur den Exit-Code von `go test`

- **Tasks:** A1, A3, A4, A5, A6 (alle Go-Grader). Art: false-pass.
- **Beschreibung:** Alle fünf entscheiden über `go test … ; rc=$?` (A1 Z. 6, A3 Z. 21, A4 Z. 6, A5 Z. 24–28,
  A6 Z. 25–28). Ob die erwarteten Testfunktionen überhaupt gelaufen sind, prüft niemand. Untracked neue
  `*_test.go` des Modells sieht `git diff <commit>` grundsätzlich nicht, und eine Nicht-Testdatei ist ohnehin
  außerhalb jedes Guards. Vier Varianten, alle mit leerem Stub (`BuildBatches … { return nil }` bzw. Bug intakt):
  (i) `func TestMain(m *testing.M) { os.Exit(0) }`; (ii) `TestMain` mit `flag.Set("test.skip","TestGrade")` und
  ehrlichem `os.Exit(m.Run())` — eigene Tests laufen grün, versteckte werden ohne SKIP-Zeile ausgefiltert;
  (iii) `syscall.Exit(0)` in einer eigenen Testfunktion — umgeht die Go-1.15-Panik bei `os.Exit` während eines
  Tests, und da Dateien alphabetisch laufen, ist `batches_test.go` vor `zz_bench_grade_test.go` dran;
  (iv) `func init() { if testing.Testing() { os.Exit(0) } }` in einer **Nicht-Testdatei** — überlebt jede
  Härtung, die modell-eigene `*_test.go` beiseitelegt. Realismus: codernext hat in A4 nachweislich Fremddateien
  ins Paketverzeichnis kopiert (`cp /tmp/test_debug.go .`, Transkript).
- **Exploit (A5, Nicht-Testdatei):**
  ```go
  package feed
  import ("os"; "testing")
  func init() { if testing.Testing() { os.Exit(0) } }
  func BuildBatches(items []RankedItem, options BatchOptions) []Batch { return nil }
  ```
  → `grade.sh`: `PASS`. Für A1/A3/A4 genügt eine untracked `zz_hijack_test.go` mit `TestMain`+`os.Exit(0)`.
- **PoC-Status:** **bestätigt (PoC-0)** für A5 mit allen vier Varianten (`poc-0/ws`, `ws-tmain`, `ws-flag`,
  `ws-sys`; Kontrolle `ws-control` → `FAIL tests-red`, `-v` zeigt null `=== RUN TestGrade*`-Zeilen bei den
  Exploits). **Bestätigt (Nachlauf)** für A6 (`a6-attacker/e2a`, `e2b` → PASS), A4 (`a4/x2-testmain`,
  `x2b-init` → PASS), A3 (`a2a3/a3-hijack` → PASS, `a3-base` → FAIL tests-red), A1 (`synth/a1-hijack` → PASS
  bei intaktem Bug, Kontrolle FAIL tests-red).
- **Patch (Kern, gilt für alle Go-Grader; vollständiges Skelett in Abschnitt 4.1):**
  ```diff
  -go test ./internal/feed/ -count=1 >/dev/null 2>&1
  -rc=$?
  -rm -f "$tgt"
  -if [ "$rc" -eq 0 ]; then echo PASS; else echo "FAIL tests-red"; exit 1; fi
  +expected=$( { (cd "$PRISTINE" && go test ./internal/feed/ -list '.*' | grep -E '^(Test|Example|Fuzz)');
  +              grep -oE '^func (Test[A-Za-z0-9_]+)\(' "$HIDDEN" | sed -E 's/^func //; s/\($//'; } | sort -u)
  +out=$(timeout --signal=SIGKILL 600 go test ./internal/feed/ -count=1 -timeout 300s -json 2>/dev/null) || true
  +passed=$(printf '%s\n' "$out" | grep -o '"Action":"pass","Package":"[^"]*","Test":"[^"/]*"' \
  +         | sed 's/.*"Test":"//; s/"$//' | sort -u)
  +pkg_ok=$(printf '%s\n' "$out" | grep -c '"Action":"pass","Package":"agora-backend/internal/feed","Elapsed"')
  +missing=$(comm -23 <(printf '%s\n' "$expected") <(printf '%s\n' "$passed"))
  +[ "$pkg_ok" -ge 1 ] && [ -z "$missing" ] && echo PASS \
  +  || { echo "FAIL tests-red missing=[$(echo $missing)]"; exit 1; }
  ```
  Damit liefern alle vier Varianten `FAIL` (null Pass-Ereignisse) — verifiziert mit `a6-attacker/grade_fixed.sh`
  (e2a/e2b → `FAIL hidden-test-not-passed`) und `a4/grade-fixed.sh` (x2/x2b → `FAIL tests-red`). Das
  `test2json`-Framing (Go ≥ 1.20, `\x16`-Marker) schützt gegen gefälschte `--- PASS`-Zeilen; das Restrisiko
  „Binary druckt den Marker selbst" deckt ein billiger Zusatz-Grep der Modell-Quellen auf `\x16`, `=== RUN`,
  `--- PASS`, `os.Exit`, `syscall.Exit`, `testing.Testing` ab (nur melden, nicht automatisch werten).

#### F2 — Tamper-Guard vertraut modell-beschreibbarem Zustand und fällt bei Git-Fehlern offen durch

- **Tasks:** A1, A2, A3, A4, A5, A6, U1, U2 (alle). Art: tamper.
- **Beschreibung:** Jeder Guard liest die Vergleichs-SHA aus `$1/.bench-baseline` und diffed mit dem `.git` des
  Workspaces — beides liegt **im** Workspace, der als `/work` rw in den Container gemountet ist (`run-task*.sh`
  schreibt die SHA nach `$RUN_DIR/ws/.bench-baseline`). Ausgewertet wird nur stdout über `| grep -q .`: schlägt
  git fehl (kein Repo, `bad revision`, leere SHA), ist stdout leer und der Guard gilt als bestanden. A1–A4 und U1
  prüfen nicht einmal, ob die Baseline-Datei existiert. Bestätigte Umgehungen, jeweils mit entschärfter
  Kept-Testdatei: (1) Test ändern, committen, neue SHA in `.bench-baseline`; (2) `.bench-baseline` löschen (auch
  als Nebenwirkung von `git clean -fdx`); (3) Inhalt `HEAD` + Commit; (4) Optionsinjektion: Inhalt
  `--output=/dev/null`; (5) `git update-index --skip-worktree`; (6) `--assume-unchanged`; (7) `git replace
  <base> HEAD`; (8) `git config diff.external /bin/true`; (9) `rm -rf .git`; (10) `commit --amend` + `reflog
  expire` + `gc --prune=now` (Baseline-Objekt weg, `.bench-baseline` unverändert); (11) Kept-Tests komplett löschen
  + Müll-Baseline. Kontrollen mit intaktem Repo liefern überall korrekt `FAIL test-file-modified`. Heute begrenzt
  sich der Schaden auf Entschärfen/Löschen behaltener Tests; sobald untracked Testdateien gehärtet werden, ist das
  der nächste Weg für eine `TestMain`-Kaperung in einer getrackten Datei.
- **Exploit (kürzeste Form, A1–A4/U1):** im Container `rm /work/.bench-baseline` — oder in allen acht Tasks
  `git -C /work update-index --assume-unchanged <kept_test>` nach beliebiger Änderung daran.
- **PoC-Status:** **bestätigt (Nachlauf)** — A4: `a4/x3-baseline`, `x3b-nobaseline`, `x3c-head`, `x4-skipwt`,
  `x4b-assume`, `x5-replace`, `x6-extdiff` → alle PASS, `synth/a4-optinj` (`--output=/dev/null`) → PASS bei
  Kontrolle FAIL test-file-modified; A5: `a5aud-git-rm`, `-assume`, `-commit`, `-gc` → PASS, `a5aud-git-ctrl` →
  FAIL; A6: `a6-attacker/e3a`, `e3a2`, `e3b`, `e3c`, `e3d`, `e3e` → PASS, `e3` (Kontrolle) → FAIL; A1: `a1t`
  (Commit + Repoint) → PASS, `synth/a1-nobase` (`rm .bench-baseline` → PASS; `HEAD`+Commit → PASS; Kontrolle
  FAIL); U2: `copies/exploit2` (Commit „model: weaken tests" + Repoint) → `PASS pass=53 fail=0`. A2 und U1 haben
  dieselbe Konstruktion wie A1 bzw. U2 und wurden nicht separat gefahren.
- **Patch:** Referenz aus dem Einflussbereich des Modells nehmen — Kept-Tests byteweise gegen den unberührten
  Baseline-Workspace, fail-closed bei jedem Fehler, kein `grep -q`-Muster:
  ```diff
  -base=$(cat "$1/.bench-baseline")
  -if git diff "$base" -- internal/feed/*_test.go | grep -q . ; then echo "FAIL test-file-modified"; exit 1; fi
  +ws=$(cd "$1" && pwd -P) || { echo "FAIL no-workspace"; exit 1; }
  +PRISTINE=/home/lowelodev/ai-lab/bench/workspaces/agora-A4-feed/agora-backend
  +for f in "$PRISTINE"/internal/feed/*_test.go; do
  +  cmp -s "$f" "$ws/agora-backend/internal/feed/$(basename "$f")" \
  +    || { echo "FAIL test-file-modified $(basename "$f")"; exit 1; }
  +done
  ```
  (A1: `gate_test.go`; A2: `tests/jsonLdScript.test.mjs`; A3: beide `livehls`-Tests; U1/U2: alle Dateien unter
  `runtime/web/test/`.) Dauerhafter als der Verweis auf `workspaces/` (das `prepare-workspaces.sh` per `rm -rf`
  neu erzeugt) ist ein einmalig erzeugtes `tasks/<task>/kept-tests.sha256`, das der Grader mit `sha256sum -c`
  prüft. Falls git bleiben soll: SHA aus `$RUN_DIR/baseline.sha` (außerhalb von `/work`), `git
  --no-replace-objects -c diff.external= diff --quiet --no-ext-diff --no-textconv "$base" -- <pfade>` und den
  **Exit-Code** auswerten (`rc != 0` → FAIL). Runner-Änderung:
  ```diff
  -git -C "$RUN_DIR/ws" rev-parse HEAD > "$RUN_DIR/ws/.bench-baseline"
  +git -C "$RUN_DIR/ws" rev-parse HEAD > "$RUN_DIR/baseline.sha"
   …
  -CHANGED=$(git -C "$RUN_DIR/ws" diff --stat "$(cat "$RUN_DIR/ws/.bench-baseline")" …)
  +CHANGED=$(git -C "$RUN_DIR/ws" diff --stat "$(cat "$RUN_DIR/baseline.sha")" …)
  ```
  Verifiziert: `a4/grade-fixed.sh` und `a6-attacker/grade_fixed.sh` melden für alle obigen Varianten
  `FAIL test-file-modified`, für legitime Lösungen PASS.

#### F3 — Kopie des versteckten Tests wird nicht geprüft: `chmod 555` auf das Paketverzeichnis ergibt PASS

- **Tasks:** A3 (Heredoc Z. 6–19), A5 (`cp` Z. 23), A6 (`cp` Z. 24). Art: false-pass.
- **Beschreibung:** `set -u` ohne `set -e`, keine Fehlerbehandlung nach `cp`/`cat >`. Schlägt das Anlegen fehl,
  läuft `go test` nur über die Bestandstests der Baseline — die kompilieren und sind grün, auch wenn
  `BuildBatches`/`RankCandidates`/die `.webm`-Regel komplett fehlen. Das Modell arbeitet mit `--userns=keep-id`
  auf dem bind-gemounteten Workspace; ein `chmod 555` im Container wirkt 1:1 auf dem Host, wo der Grader als
  derselbe Nutzer läuft. Die Symlink-Variante (toter Symlink am Zielpfad) schließt sich selbst: Go bricht den
  Build ab → FAIL.
- **Exploit:** letzte Aktion im Container: `chmod 555 /work/agora-backend/internal/feed` (A5/A6) bzw.
  `…/internal/livehls` (A3). Keine Codeänderung nötig.
- **PoC-Status:** **bestätigt (PoC-1)** für A5 (`poc-1/ws`, unveränderte Baseline ohne Batcher: `cp: … Permission
  denied` und dann `PASS`; Kontrolle FAIL tests-red). **Bestätigt (PoC-3)** für A6 (`poc-3/reproduce.sh`: Stub,
  der jede Property verletzt, + `a-w` → PASS). **Bestätigt (Nachlauf)** für A3 (`synth/a3-chmod`: `line 6: …
  Permission denied` → `PASS`, Kontrolle FAIL tests-red). Für U2 **verworfen**: `synth/u2-chmod` → `FAIL
  too-few-passing pass=46 need=52`, der Zähler fällt geschlossen. Symlink-Variante (`a5aud-symlink`) → FAIL,
  verworfen.
- **Patch:** in einer Wegwerf-Kopie bewerten, Kopie verifizieren, Heredoc in eine Datei unter `tasks/` auslagern:
  ```diff
  -cp /home/lowelodev/ai-lab/bench/tasks/agora-A5-batcher-scratch/grade_test.go "$tgt"
  +tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
  +cp -a "$ws/agora-backend" "$tmp/agora-backend" && chmod -R u+w "$tmp/agora-backend" \
  +  || { echo "FAIL copy-workspace"; exit 1; }
  +tgt="$tmp/agora-backend/internal/feed/zz_bench_grade_test.go"
  +[ -e "$tgt" ] && { echo "FAIL grade-file-collision"; exit 1; }
  +cp "$HIDDEN" "$tgt" && cmp -s "$HIDDEN" "$tgt" || { echo "FAIL grade-file-copy"; exit 1; }
  ```
  Die Pass-Ereignis-Prüfung aus F1 fängt den Fall ebenfalls (keine `TestGrade*`-Events); der explizite Check
  macht die Ursache sichtbar. Generell `set -euo pipefail` in allen Gradern.

#### F4 — Node-Summary-Injection: `head -1` nimmt die erste `ℹ pass`-Zeile, und die kommt aus dem Modell-Code

- **Tasks:** A2 (Z. 7–9), U1 (Z. 7–9). Art: false-pass. U2 ist **nicht** betroffen (siehe unten).
- **Beschreibung:** `node --test` führt Testdateien in Kindprozessen aus und reicht deren stdout **vor** der
  eigenen Zusammenfassung durch. `grep -oP 'ℹ pass \K\d+' | head -1` greift die erste passende Zeile — und die
  kann aus `src/index.js` bzw. dem Helper stammen, den der Test importiert. U2 parst verankert (`^ℹ pass N$`)
  und nimmt `tail -n 1`: dort gewinnt immer die echte Zusammenfassung des Elternprozesses (Node 26.7:
  Kind-Monkeypatch von `process.stdout.write`, `fs.writeSync(1, …)` im `exit`-Handler und gefälschte TAP-Zeilen
  ändern sie nicht; `nodefmt/*`, `exp/*`).
- **Exploit (U1, Bug intakt):** zwei Zeilen am Anfang von `runtime/web/src/index.js`:
  `console.log('ℹ pass 46'); console.log('ℹ fail 0');` → `PASS`. A2: Helper ohne jede Escaping-Logik plus
  `console.log("ℹ pass 4"); console.log("ℹ fail 0");` → `PASS`.
- **PoC-Status:** **bestätigt (Nachlauf)** — `synth/u1-inject` (Kontrolle `FAIL tests (pass=45 fail=1)`, Exploit
  `PASS`, `offset + limit - 1` unverändert); `a2a3/a2-inject` → `PASS` mit ungeschütztem
  `JSON.stringify(value)`. Gegenprobe mit U2-Parser auf a2-inject: `pass=2 fail=2`, `node rc=1` — korrekt.
- **Patch (A2 und U1, wie U2 plus Exit-Code):**
  ```diff
  -out=$(node --test tests/jsonLdScript.test.mjs 2>&1)
  -pass=$(echo "$out" | grep -oP 'ℹ pass \K\d+' | head -1)
  -fail=$(echo "$out" | grep -oP 'ℹ fail \K\d+' | head -1)
  -[ "${fail:-1}" = "0" ] && [ "${pass:-0}" -ge 4 ] || { echo "FAIL tests-red (pass=$pass fail=$fail)"; exit 1; }
  +out=$(node --test tests/jsonLdScript.test.mjs 2>&1); rc=$?
  +pass=$(printf '%s\n' "$out" | sed -n 's/^ℹ pass \([0-9][0-9]*\)$/\1/p' | tail -n 1)
  +fail=$(printf '%s\n' "$out" | sed -n 's/^ℹ fail \([0-9][0-9]*\)$/\1/p' | tail -n 1)
  +[ -n "$pass" ] && [ -n "$fail" ] || { echo "FAIL no-test-summary"; exit 1; }
  +[ "$rc" -eq 0 ] && [ "$fail" -eq 0 ] && [ "$pass" -eq 4 ] \
  +  || { echo "FAIL tests-red (pass=$pass fail=$fail rc=$rc)"; exit 1; }
  ```
  (U1: `-eq 46`.) Zusätzlich in U2 den `rc` mitprüfen.

#### F5 — A6: keine Permutations-/ID-Swap-Verteidigung — reiner ID-Sort besteht alle Properties

- **Task:** A6. Art: false-pass.
- **Beschreibung:** Dieselbe Lochklasse wie beim muse-Präzedenzfall in A5, für A6 nicht gefixt. Alle acht
  paarweisen ceteris-paribus-Properties übergeben den Verlierer als `candidates[0]` = `zzBenchDebate(1)` und den
  Gewinner als `candidates[1]` = `zzBenchDebate(2)`: „Gewinner" korreliert perfekt mit höherer ID und zweiter
  Eingabeposition. Ein Scorer, der `ModeNew` nach `CreatedAt`, `ModeTop` nach `QualityScore` und sonst stur nach
  `ID` absteigend sortiert, verletzt praktisch den gesamten Brief (keine semantische Nähe, kein Follow, keine
  Affinität, keine Frische, kein Live-Nudge, keine Suggestion-Eindämmung) und besteht alle zehn Properties.
- **Exploit:** `a6-attacker/idsort.go` als einzige `scorer.go`; Variante `revsort.go` (Eingabe umkehren).
- **PoC-Status:** **bestätigt (PoC-2)** — `poc-2/ws` (ID-Sort) → `PASS`, alle zehn `TestZZBench*` grün;
  `poc-2/ws-revsort` → `PASS`; Kontrolle `ws-control-noop` (Eingabereihenfolge) → `FAIL tests-red` mit neun roten
  Properties. Nachlauf: `a6-attacker/e1`, `e1b` → PASS.
- **Patch:** `grade_test.go` so härten, dass jede Paar-Property unter beiden Eingabe-Reihenfolgen **und** beiden
  ID-Zuordnungen läuft, die 3er-`ModeNew`-Property unter allen 6 Permutationen × 2 ID-Zuordnungen. Umgesetzt in
  `a6-attacker/grade_test_patched.go` (Helfer `zzBenchPair` mit vier Varianten, `zzBenchPermutations(3)`).
  Validierung im Nachlauf (`grade_orig_patchedtest.sh` über `a6-attacker/legit-*`): alle 9 PASS-Lösungen
  (agy-37flash, cc-opus48, cc-opus5, codernext-vulkan, dsh-v4-flash, dsh-v4-pro, oc-gemini37f, oc-v4-flash,
  oc-v4-pro) bleiben PASS, muse und qwen36moe bleiben FAIL, e1/e1b kippen auf FAIL. **Kein Urteil ändert sich.**

#### F6 — A4: Glob-Expansion durch bash — gelöschte oder umbenannte Testdatei ist für den Guard unsichtbar

- **Task:** A4 (Z. 5). Art: false-pass.
- **Beschreibung:** `git diff "$base" -- internal/feed/*_test.go` — bash expandiert den Glob über den
  **aktuellen** Working Tree. Eine gelöschte oder umbenannte Datei ist nicht mehr im Glob und wird nie gediffed;
  solange eine andere `*_test.go` übrig bleibt, ist die Ausgabe leer. `batcher_test.go` trägt den einzigen roten
  Test und wird von keiner anderen Testdatei gebraucht (Helfer lokal).
- **Exploit:** `rm internal/feed/batcher_test.go` — sonst nichts, Bug bleibt.
- **PoC-Status:** **bestätigt (Nachlauf)** — `a4/x1-delete`, `x1b-gitrm`, `x1c-underscore`
  (`_batcher_test.go`, von Go ignoriert), `x1d-bak` → alle `PASS`; `a4/grade-fixed.sh` → `FAIL
  test-file-modified batcher_test.go` für alle vier; Kontrollen `ctrl-fixed` PASS / `ctrl-unfixed` FAIL in beiden.
- **Patch:** Minimal — Pathspec quoten, damit git den Glob auswertet und Löschungen sieht:
  ```diff
  -if git diff "$base" -- internal/feed/*_test.go | grep -q . ; then
  +if git diff "$base" -- 'internal/feed/*_test.go' | grep -q . ; then
  ```
  Vollständig: pristine-`cmp` aus F2 (prüft Existenz und Inhalt). Entwurf: `a4/grade-fixed.sh`.

#### F7 — A2: Verdrahtungsprüfung ist ein negativer Literal-Grep

- **Task:** A2 (Z. 10). Art: false-pass.
- **Beschreibung:** `grep -q 'JSON.stringify(jsonLd)' page.tsx → FAIL not-wired-into-page` prüft nur die
  Abwesenheit einer exakten Zeichenkette. `JSON.stringify(jsonLd, null, 0)`, `JSON.stringify(jsonLd as object)`
  oder eine Zwischenvariable lassen den ungeschützten Aufruf am realen Call-Site stehen; der Helper existiert,
  die Spec ist grün → PASS, obwohl der Breakout in Produktion bleibt — der eigentliche Zweck der Aufgabe.
- **Exploit:** `a2a3/a2-notwired`: korrekter Helper in `src/lib/jsonLdScript.ts`, aber
  `__html: JSON.stringify(jsonLd, null, 0)` in der Seite.
- **PoC-Status:** **bestätigt (Nachlauf)** → `PASS`; Baseline `a2-base` → `FAIL tests-red`.
- **Patch:** positive Prüfung auf den Helper-Aufruf in der `ld+json`-Zeile (Zeilenumbrüche vorher glätten) plus
  Negativ-Prüfung ohne schließende Klammer:
  ```diff
  -if grep -q 'JSON.stringify(jsonLd)' 'src/app/debate/[id]/page.tsx'; then echo "FAIL not-wired-into-page"; exit 1; fi
  +page='src/app/debate/[id]/page.tsx'
  +tr -s ' \n\t' ' ' < "$page" | grep -q 'application/ld+json[^<]*__html: *jsonLdScriptBody(' \
  +  || { echo "FAIL not-wired-into-page"; exit 1; }
  +grep -q 'JSON.stringify(jsonLd' "$page" && { echo "FAIL not-wired-into-page"; exit 1; }
  ```
  Vorabprüfung über alle 12 A2-Runs: `stringify=0`, Helper-Aufruf in der ld+json-Zeile = 1, Import vorhanden —
  kein Urteil ändert sich.

#### F8 — A5 Property (c) verlangt defensives Selbst-Sortieren, das Brief und Aufrufer-Vertrag nicht fordern

- **Task:** A5. Art: false-fail (12/12 Einträge betroffen).
- **Beschreibung:** `TestGradeMobileOneItemPerBatchBestFirst` (grade_test.go Z. 133–164) verlangt für alle 24
  Anordnungen, dass die erste Handy-Seite das Score-Maximum trägt — `BuildBatches` muss also selbst nach Score
  ordnen. Der Brief sagt „Die Rangfolge der Beiträge haben wir schon … eine fertig bewertete, sortierbare Liste"
  und verweist auf den übrigen Code; der liefert sortiert (`scorer.go:34` `sort.SliceStable` in `RankCandidates`,
  `handlers/feed.go:172–190` reicht genau dieses `ranked` ordnungserhaltend weiter). Der einzige Hinweis auf die
  Gegenlesart ist das Wort „sortierbare". Sämtliche Cloud-/Frontier-Modelle (dsh-v4-flash/pro, oc-gemini37f,
  oc-v4-flash/pro, cc-opus48, cc-opus5, agy-37flash) scheitern ausschließlich an dieser Property; cc-opus5
  dokumentiert die Lesart explizit („groups an already-ranked feed page"). Die Batterie (Abschnitt 3) zeigt
  dieselbe Schwäche flächendeckend, keine weitere.
- **Exploit:** eine legitime Lösung, die die Eingabeordnung als Rangfolge übernimmt und Themen-Mix, Live-Kadenz,
  Pacing und Konservierung korrekt umsetzt (z. B. cc-opus5/`batching.go`) → `FAIL tests-red`.
- **PoC-Status:** **bestätigt (Nachlauf, Doppel-Wertung)** — `synth/a5-double-score.sh` über alle 12 Abgaben
  plus Referenz (Original-Batcher aus dem A6-Workspace):

  | Label | aktueller Grader | V1: (c) Vertragslesart | V2: V1 + (b) entschärft + (e2) |
  |---|---|---|---|
  | Referenz (Original-Batcher) | PASS | PASS | PASS |
  | agy-37flash | FAIL [(c)] | **PASS** | **PASS** |
  | cc-opus48 | FAIL [(c)] | **PASS** | **PASS** |
  | cc-opus5 | FAIL [(c)] | **PASS** | **PASS** |
  | dsh-v4-flash | FAIL [(c)] | **PASS** | **PASS** |
  | dsh-v4-pro | FAIL [(c)] | **PASS** | **PASS** |
  | oc-gemini37f | FAIL [(c)] | **PASS** | **PASS** |
  | oc-v4-flash | FAIL [(c)] | **PASS** | **PASS** |
  | oc-v4-pro | FAIL [(c)] | **PASS** | **PASS** |
  | codernext-vulkan | FAIL [(c),(d),(e)] | FAIL [(d),(e)] | FAIL [(c),(d),(e),(e2)] |
  | muse-vulkan | FAIL [(c)] | FAIL [(c)] | FAIL [(c),(e2)] |
  | qwen36moe-vulkan | FAIL [undefined: BuildBatches] | FAIL | FAIL |
  | qwen38-vulkan | FAIL [undefined: BuildBatches] | FAIL | FAIL |

  Bemerkenswert: muse fällt auch unter der Vertragslesart (sortierte Eingabe) durch (c) — die Aberkennung des
  muse-PASS war unter **beiden** Lesarten richtig.
- **Patch (V1, `synth/a5-grade-v1_test.go`):**
  ```diff
  -	gradePermute(len(base), func(order []int) {
  -		items := … permutiert …
  +	// Vertrag des Aufrufers: RankCandidates liefert absteigend sortiert.
  +	items := []RankedItem{ gradeItem(2, …, 90, …), gradeItem(3, …, 70, …), gradeItem(4, ItemLiveRoom, 60, …), gradeItem(1, …, 40, …) }
   	batches := BuildBatches(items, mobileOptions)
   	// unverändert: len(batches) == len(items), je genau 1 Item, batches[0] == Score-Maximum
  ```
  Alternative: Brief um einen Satz ergänzen („Die Liste kann in beliebiger Reihenfolge ankommen; maßgeblich ist
  ausschließlich der Score") und (c) streng lassen — dann aber nur für künftige Kampagnen, die bisherigen Läufe
  sahen den mehrdeutigen Brief. Entscheidung siehe Abschnitt 4.3.

### Schwere MITTEL

#### F9 — A5 Property (e): Implementierung ohne Konsekutiv-Top-Slot-Logik besteht

- **Task:** A5. Art: false-pass.
- **Beschreibung:** `TestGradeNoBackToBackLiveTopSlotOutsideLiveMode` prüft ein einziges Fixture (L100, T95,
  L94, L93, T50; PageSize 3). Ein „höchstens ein Live-Raum pro Seite"-Design erfüllt es zufällig: Seite 1 =
  [L100, T95, T50], Seite 2 = [L94, L93] ist erzwungener All-Live-Rest und fällt unter die Toleranz. Die
  Brief-Regel ist nirgends umgesetzt. Gegen-Fixture L100, T95, T94, L93, T92, T91: Seite 1 top=live, Seite 2
  top=live bei vier verfügbaren Text-Beiträgen.
- **PoC-Status:** **bestätigt (Nachlauf)** — `a5aud-livecap` → `PASS` unter dem installierten Grader; unter V2
  (`(e2)` permutiert, `synth/a5-grade-v2_test.go`) fällt es; die Referenz besteht V2, und keine der acht unter V1
  bestehenden Abgaben reißt (e2) — **kein Urteilseffekt**, nur Lochschluss.
- **Patch:** `TestGradeNoBackToBackLiveTopSlotWhenTextIsPlentiful` mit `gradePermute(6, …)` (Text in V2-Datei).
  Nicht auf Mobile ausdehnen (Original-Batcher setzt die Kadenz nur für Feature-Layouts um).

#### F10 — Relativer Aufrufpfad deaktiviert den Guard (A1–A4, U1) bzw. bricht den Grader (A5, A6, U2)

- **Tasks:** alle. Art: infra / tamper-Nebenwirkung.
- **Beschreibung:** Jeder Grader macht `cd "$1/…"` **bevor** er `cat "$1/.bench-baseline"` liest. Mit relativem
  `$1` (Handregrade `bash grade.sh runs/x/y/ws`) ist der Pfad danach ungültig: A5/A6/U2 melden `FAIL
  no-baseline`, A1–A4/U1 lesen eine leere SHA, der Guard fällt offen durch (F2) und eine modifizierte Testdatei
  wird nicht bemerkt.
- **PoC-Status:** **bestätigt (Nachlauf)** — `a4/x7-relpath` (Test entschärft, relativer Pfad) → `PASS`;
  `audit-scratch/cheat` relativ → `FAIL no-baseline`, absolut → `PASS pass=52`.
- **Patch:** `ws=$(cd "$1" && pwd -P) || { echo "FAIL no-workspace"; exit 1; }` als erste Zeile, danach nur
  `$ws`. Mit der pristine-Referenz aus F2 entfällt die Baseline-Datei ohnehin.

### Schwere NIEDRIG

#### F11 — Kein Timeout um `go test`: Endlosschleife in `init()` hängt den Grader unbegrenzt

- **Tasks:** A1, A3, A4, A5, A6. Art: infra (DoS auf die Bewertung, kein PASS-Gewinn).
- **Beschreibung:** Gos `-timeout`-Watchdog greift erst, wenn Testfunktionen laufen; ein `for {}` in einem
  package-level `init()` liegt davor. Die Runner umschließen nur `podman run` mit `timeout`, nicht den
  Host-Grade-Schritt. Ein unbeaufsichtigter Nachtlauf bliebe hängen.
- **PoC-Status:** **bestätigt (Nachlauf)** — `a6-attacker/inithang`: `timeout 5 go test -timeout 2s .` → `rc=124`
  (äußerer timeout), Go-Watchdog feuerte nicht.
- **Patch:** `timeout --signal=SIGKILL 600 go test … -timeout 300s` und `rc=124` als `FAIL timeout` werten
  (in F1-Diff enthalten); den Grade-Schritt in `run-task*.sh` ebenfalls unter `timeout 900` stellen.

#### F12 — A5 Property (b) fordert null Batches, Brief fordert nur „nicht abstürzen"

- **Task:** A5. Art: false-fail (theoretisch).
- **Beschreibung:** Assertion `len(batches) != 0` ist strenger als ihre eigene Begründung („no batches with
  content"). Ein Leerzustands-Design (`[]Batch{{ID: "for_you-1", Layout: LayoutSingle}}`) ist brief-konform
  und bekommt FAIL. Keine Abgabe betroffen.
- **PoC-Status:** **bestätigt (Nachlauf)** — `a5aud-empty` → `FAIL tests-red`.
- **Patch:** in V2 enthalten:
  ```diff
  -	if batches := BuildBatches(nil, gradeDesktopOptions(3)); len(batches) != 0 {
  -		t.Fatalf("nil input produced %d batches, want none", len(batches))
  +	for _, batch := range BuildBatches(nil, gradeDesktopOptions(3)) {
  +		if len(batch.Items) != 0 {
  +			t.Fatalf("nil input produced a batch with %d items, want no content", len(batch.Items))
  +		}
   	}
  ```

#### F13 — U2: versteckte Tests prüfen Anforderung 1 („jedes Tool") nur für zwei Tools

- **Task:** U2. Art: Abdeckungslücke (nicht ausnutzbar, da Tests verdeckt).
- **Beschreibung:** `grade.test.js` treibt `denyTools` nur mit `payload.list` und
  `runtime.search_types`/`runtime.search_classes`. Eine Implementierung, die die Sperre nur für genau diese
  Namen im Chokepoint auswertet, besteht.
- **PoC-Status:** **bestätigt (Nachlauf)** — `audit-scratch/cheat` (Sperre nur für die zwei Tools) → `PASS
  pass=52 fail=0`.
- **Patch:** eine Tabelle über mehrere Tools (je eines aus `runtime.*`, `ui.*`, `payload.*`, `code.*`) und
  zusätzlich ein zur Laufzeit aus `runtime.capabilities().tools` gewähltes Tool sperren und den
  `RuntimeSecurityError` verlangen.

#### F14 — A2: Node-Type-Stripping lässt nicht-löschbare TS-Syntax im Helper scheitern

- **Task:** A2. Art: false-fail (gering, selbst erkennbar).
- **Beschreibung:** Die Spec importiert `../src/lib/jsonLdScript.ts` direkt in Node; ein `enum`,
  Parameter-Properties oder `namespace` im Helper sind unter Type-Stripping ein SyntaxError, obwohl Next.js sie
  baut. Da der Brief das exakte Kommando vorgibt, sieht das Modell den Fehler selbst.
- **PoC-Status:** **bestätigt (Nachlauf)** — `a2a3/a2-enum` → `FAIL tests-red (pass=0 fail=1)`.
- **Patch:** Hinweis im Brief („nur löschbare TS-Syntax, Node lädt die Datei direkt") oder — falls Node 26 es
  anbietet — `--experimental-transform-types` im Grader-Aufruf.

### Verworfene Angriffe (zur Vollständigkeit)

- U2, `chmod 555 runtime/web/test` → `FAIL too-few-passing pass=46 need=52` (Zähler fällt geschlossen).
- A5, toter Symlink am Zielpfad des versteckten Tests → Go-Build bricht ab, `FAIL tests-red`.
- Node-Summary-Injection gegen den U2-Parser (Monkeypatch von `stdout.write`, direkter `fd 1`-Write im
  `exit`-Handler, gefälschte TAP-Zeilen) → echte Zusammenfassung gewinnt.

### Rückschau auf die echten Abgaben (Vorabprüfung für die Regrades)

- Hijack-Muster (`TestMain(`, `os.Exit(`, `syscall.Exit(`, `testing.Testing()`, `test.skip`, `test.run`) in
  `internal/{feed,aiwork,livehls}` aller Runs: **kein Treffer**; `ℹ pass|fail`, `process.exit`,
  `process.stdout.write =` in `runtime/web/src` und `agora-web/src`: **kein Treffer**.
- Rechte aller Paket- und Testverzeichnisse: **alle 755**.
- 400 behaltene Testdateien über 98 Runs byteweise gegen `workspaces/`: **2 Abweichungen**, beide bereits als
  `FAIL test-file-modified` gewertet (codernext-vulkan/A3 `cachepolicy_test.go`, qwen36moe-vulkan/U2
  `adapter.test.js`).
- A2-Verdrahtung: alle 12 Runs rufen den Helper in der `ld+json`-Zeile, kein `JSON.stringify(jsonLd` mehr.

---

## 3. Robustheits-Batterie (Zusatz-Metrik, ändert keine Urteile)

### A5 — `robustness-battery/a5-results.md`

Batterie: 12 Tests (7 ZZBatReal\*, 5 ZZBatPath\*), nur Brief-Invarianten; Workspaces nach dem Lauf byte-identisch.

| Label | Real x/y | Path x/y | Bemerkung |
|---|---|---|---|
| agy-37flash | 6/7 | 5/5 | übernimmt Eingabe-Reihenfolge (kein Sortieren); reißt Ties-Mobile bei umgekehrter Anordnung (Seite 1: Score 60 statt 90) |
| cc-opus48 | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 bei Anordnung [3 2 1 0] |
| cc-opus5 | 6/7 | 5/5 | dito; Gegencheck: reißt aus demselben Grund auch die aktuelle Grade-Property (c) schon bei Identitäts-Anordnung |
| dsh-v4-flash | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| dsh-v4-pro | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| muse-vulkan | 6/7 | 5/5 | dito, mit eigener Färbung: Live-Kadenz-Heuristik zieht bei umgekehrter Anordnung das Live-Item vor (Seite 1: Score 70 statt 90) |
| oc-gemini37f | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| oc-v4-flash | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| oc-v4-pro | 6/7 | 5/5 | dito — Seite 1: Score 60 statt 90 |
| qwen36moe-vulkan | nicht baubar | nicht baubar | gegradete API fehlt: `undefined: BuildBatches` (kein Beitrag im feed-Paket) |
| qwen38-vulkan | nicht baubar | nicht baubar | gegradete API fehlt: `undefined: BuildBatches` (nur go.mod angefasst) |

Alle 9 baubaren Lösungen reißen exakt einen Test, denselben: `TestZZBatRealScoreTiesMobileLeadsWithTopScore`
(Pool 90, 90, 70-live, 60 über 4 Anordnungen; bei `[3 2 1 0]` liefert jede Lösung das erste Eingabe-Item).
codernext-vulkan war zum Batterie-Zeitpunkt noch nicht gelaufen.

### A6 — `robustness-battery/a6-results.md` (v2)

Batterie: 21 Tests (12 ZZBatReal\*, 9 ZZBatPath\*), Watchdog je Aufruf, Workspaces nach dem Lauf byte-identisch.

| Label | Real x/y | Path x/y | Bemerkung |
|---|---|---|---|
| agy-37flash | 12/12 | 9/9 | offiziell PASS; keine Risse |
| cc-opus48 | 12/12 | 9/9 | offiziell PASS; keine Risse (v1 hatte den Workspace noch nicht — Lauf war damals aktiv) |
| cc-opus5 | 12/12 | 9/9 | offiziell PASS; keine Risse |
| codernext-vulkan | 11/12 | 9/9 | offiziell PASS; ein Riss, nur strengste Lesart: Zeitstempel-Ties in „neu" folgen der Eingabereihenfolge (kein Tiebreak in der Chronologie; „für dich"/„top" haben ID-Tiebreak) |
| dsh-v4-flash | 12/12 | 9/9 | offiziell PASS; keine Risse |
| dsh-v4-pro | 12/12 | 9/9 | offiziell PASS; keine Risse |
| muse-vulkan | 9/12 | 9/9 | offiziell FAIL tests-red; „neu" ist gewichtete Mischung (Frische 0,45 + Personalisierung), nicht chronologisch — 3 Risse, dasselbe Muster wie Grade-Property f1 |
| oc-gemini37f | 12/12 | 9/9 | offiziell PASS; keine Risse |
| oc-v4-flash | 12/12 | 9/9 | offiziell PASS (nach Regrade); keine Risse |
| oc-v4-pro | 12/12 | 9/9 | offiziell PASS (nach Regrade); keine Risse |
| qwen36moe-vulkan | 8/12 | 9/9 | offiziell FAIL tests-red; „neu" sortiert nach Score, CreatedAt nur als Tiebreak (3 Chronologie-Risse) + kein ID-Tiebreak (Ties folgen der Eingabereihenfolge) |
| qwen38-vulkan | nicht baubar | — | keine Abgabe: Paket baut ohne Batterie (base_rc=0), mit Batterie `undefined: RankCandidates`; Nachzügler-Retry mit 64k Kontext steht laut `nachzuegler-a5a6-retry.sh` noch aus |

### Deutung

Referenzpunkt ist jeweils die beste Lösung: in A6 die acht mit 12/12 + 9/9 (agy-37flash, cc-opus48, cc-opus5,
dsh-v4-flash, dsh-v4-pro, oc-gemini37f, oc-v4-flash, oc-v4-pro), in A5 sind alle neun baubaren Lösungen mit
6/7 + 5/5 gleichauf. Gegenüber dieser Spitze liegt codernext in A6 nur einen Riss unter der strengsten
Determinismus-Lesart zurück (Ties in „neu"), muse drei und qwen36moe vier Risse — und beide letzteren aus genau
dem Grund, aus dem sie offiziell durchgefallen sind („neu" nicht chronologisch), die Batterie widerspricht also
keinem Urteil, sie schärft es. In A5 ist die einzige Schwäche über alle neun Lösungen dieselbe Sortier-Frage wie
in Property (c) — kein zusätzliches Loch, sondern eine unabhängige Bestätigung, dass es sich um eine Spec-Lesart
und nicht um Modellschwäche handelt (F8). Die pathologische Stufe (NaN, ±Inf, MaxInt, Duplikat-IDs, PageSize 0/
negativ, 3000er-Pool) bestehen 9/9 (A5) und 11/11 (A6) baubare Lösungen ohne Panic, Verlust oder Hänger; die
Eingabe-Hygiene ist durch die Bank gut. Die Batterie bleibt Zusatz-Metrik für das Dashboard und ändert keine
PASS/FAIL-Urteile.

---

## 4. Anwendung

**Zeitpunkt:** erst nach `NACHTKETTE-2 KOMPLETT` **und** `A5A6-RETRY KOMPLETT` (beide Ketten laufen noch:
`nachtkette-2.sh` Phase 4 Polyglot, `nachzuegler-a5a6-retry.sh` wartet und wird danach qwen38 A5/A6 und
qwen36moe A5 mit `run-task.sh` neu fahren — der ruft `grade.sh` zur Laufzeit). Grader während laufender
Suite-Läufe zu tauschen würde die Kette mit gemischten Gradern bewerten. Die Polyglot-/Claude-Polyglot-Läufe
nutzen diese Grader nicht und stören nicht.

### 4.1 Patch-Pakete (Empfehlungsliste, in Reihenfolge)

| # | Paket | Betrifft | Inhalt | Schließt |
|---|---|---|---|---|
| P1 | **Go-Grader-Skelett** | A1, A3, A4, A5, A6 | Wegwerf-Kopie (`u+w`), verifizierte Kopie des versteckten Tests, pristine-`cmp`-Guard, `go test -json` unter `timeout`, PASS nur bei Paket-pass **und** Pass-Ereignis je erwartetem Test (Baseline-Tests aus `go test -list` + Namen aus der Hidden-Datei), `set -euo pipefail`, absoluter Pfad. A3-Heredoc nach `tasks/agora-A3-hls/grade_test.go` auslagern. Entwürfe: `audit-scratch/a4/grade-fixed.sh`, `audit-scratch/a6-attacker/grade_fixed.sh`. | F1, F2, F3, F6, F10, F11 |
| P2 | **Node-Grader** | A2, U1, U2 | verankerter `tail -1`-Parser + `rc`-Prüfung, pristine-`cmp`-Guard über `tests/…` bzw. `runtime/web/test/`, absoluter Pfad, `set -euo pipefail`; A2 zusätzlich positive Verdrahtungsprüfung. | F2, F4, F7, F10 |
| P3 | **Runner** | `run-task*.sh` | Baseline-SHA nach `$RUN_DIR/baseline.sha` statt in `/work`; Grade-Schritt unter `timeout 900`; optional `tasks/<task>/kept-tests.sha256` erzeugen und in P1/P2 statt `workspaces/` referenzieren. | F2, F11 |
| P4 | **A6 grade_test.go** | A6 | `audit-scratch/a6-attacker/grade_test_patched.go` installieren (Paar-Properties unter beiden Reihenfolgen × beiden ID-Zuordnungen, ModeNew über 6 × 2). | F5 |
| P5 | **A5 grade_test.go V2** | A5 | `audit-scratch/synth/a5-grade-v2_test.go`: (c) Vertragslesart, (b) entschärft, (e2) Live-Kadenz bei reichlich Text permutiert. **Entscheidung nötig (4.3).** | F8, F9, F12 |
| P6 | **U2 grade.test.js** | U2 | Tool-Tabelle + Laufzeit-gewähltes Tool. | F13 |
| P7 | Brief A2 / Grader-Flag | A2 | Satz zu löschbarer TS-Syntax oder Transform-Flag. | F14 |
| P8 | Defense-in-depth-Grep | alle Go | Modell-Quellen auf `\x16`, `=== RUN`, `--- PASS`, `os.Exit`, `syscall.Exit`, `testing.Testing` — nur als Warnzeile im Grade-Log. | Restrisiko F1 |

Referenz-Skelett für P1 (A5 als Beispiel; A1/A4 ohne `HIDDEN`, A3 mit ausgelagerter Datei, Paketpfad je Task):

```bash
#!/usr/bin/env bash
set -euo pipefail
BENCH=/home/lowelodev/ai-lab/bench; TASK=agora-A5-batcher-scratch; PKG=internal/feed
HIDDEN=$BENCH/tasks/$TASK/grade_test.go            # leer lassen bei A1/A4
PRISTINE=$BENCH/workspaces/$TASK/agora-backend
ws=$(cd "$1" && pwd -P) || { echo "FAIL no-workspace"; exit 1; }
[ -d "$ws/agora-backend/$PKG" ] || { echo "FAIL no-workspace"; exit 1; }
# 1 Tamper-Guard gegen die unberührte Baseline (Existenz + Inhalt)
for f in "$PRISTINE/$PKG"/*_test.go; do
  cmp -s "$f" "$ws/agora-backend/$PKG/$(basename "$f")" || { echo "FAIL test-file-modified $(basename "$f")"; exit 1; }
done
# 2 Wegwerf-Kopie, versteckten Test verifiziert einkopieren
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
cp -a "$ws/agora-backend" "$tmp/agora-backend"; chmod -R u+w "$tmp/agora-backend"
if [ -n "${HIDDEN:-}" ]; then
  tgt="$tmp/agora-backend/$PKG/zz_bench_grade_test.go"
  [ -e "$tgt" ] && { echo "FAIL grade-file-collision"; exit 1; }
  cp "$HIDDEN" "$tgt" && cmp -s "$HIDDEN" "$tgt" || { echo "FAIL grade-file-copy"; exit 1; }
fi
# 3 Erwartete Tests: Baseline (nur Listen, nicht Ausführen) + versteckte
expected=$( { (cd "$PRISTINE" && go test "./$PKG/" -list '.*' 2>/dev/null | grep -E '^(Test|Example|Fuzz)' || true);
              [ -n "${HIDDEN:-}" ] && grep -oE '^func (Test[A-Za-z0-9_]+)\(' "$HIDDEN" | sed -E 's/^func //; s/\($//'; } | sort -u)
[ -n "$expected" ] || { echo "FAIL no-expected-tests"; exit 1; }
# 4 Lauf unter Timeout; PASS nur bei Paket-pass UND Pass-Ereignis je erwartetem Test
out=$(cd "$tmp/agora-backend" && GOFLAGS="-mod=mod -buildvcs=false" GOPROXY=off \
      timeout --signal=SIGKILL 600 go test "./$PKG/" -count=1 -timeout 300s -json 2>/dev/null) && rc=0 || rc=$?
[ "$rc" -eq 124 ] || [ "$rc" -eq 137 ] && { echo "FAIL timeout"; exit 1; }
passed=$(printf '%s\n' "$out" | grep -o '"Action":"pass","Package":"[^"]*","Test":"[^"/]*"' | sed 's/.*"Test":"//; s/"$//' | sort -u || true)
pkg_ok=$(printf '%s\n' "$out" | grep -c "\"Action\":\"pass\",\"Package\":\"agora-backend/$PKG\",\"Elapsed\"" || true)
missing=$(comm -23 <(printf '%s\n' "$expected") <(printf '%s\n' "$passed"))
if [ "${pkg_ok:-0}" -ge 1 ] && [ -z "$missing" ]; then echo PASS; else echo "FAIL tests-red missing=[$(echo $missing)]"; exit 1; fi
```

Modell-eigene Testdateien bleiben dabei im Lauf (Brief: „die übrigen Tests des Pakets müssen weiterhin
durchlaufen"); ihre Kaperungsmöglichkeiten sind über die Pass-Ereignisse neutralisiert. Der A4-Entwurf entfernt
Fremdtests stattdessen — das ändert die Semantik (`a4/n1-workaround`: Workaround-Fix + eigener roter Probe-Test
wäre dann PASS statt FAIL) und wird **nicht** empfohlen.

### 4.2 Regrades und erwartete Folgen

Nach Installation der Pakete werden **alle letzten Einträge je (Label, Task)** neu bewertet — 98 Bewertungen
(A1 13, A2 12, A3 12, A4 12, A5 12, A6 12, U1 13, U2 12), plus die Nachzügler-Retries, sobald sie eingetragen
sind. Erwartete Verschiebungen laut Vorabprüfung:

| Paket | Regrade | erwartete Flips | Grundlage |
|---|---|---|---|
| P1 + P3 | alle 61 Go-Einträge | **0** | keine Hijack-Muster, alle Verzeichnisse 755, 400 Kept-Tests byteweise identisch bis auf 2 bereits-FAIL-Fälle; A6-Härtung über 11 Kopien 0 Flips |
| P2 | alle 37 Node-Einträge | **0** | kein `ℹ`/`process.exit` in Modell-Quellen, A2-Verdrahtung in 12/12 positiv, U2-Kept-Tests identisch bis auf qwen36moe (bereits FAIL) |
| P4 | A6 (12) | **0** | validiert über alle 11 Abgaben: 9 PASS bleiben, muse/qwen36moe bleiben FAIL, qwen38 ohne Abgabe |
| P5 | A5 (12) | **8 FAIL → PASS** (agy-37flash, cc-opus48, cc-opus5, dsh-v4-flash, dsh-v4-pro, oc-gemini37f, oc-v4-flash, oc-v4-pro); codernext, muse, qwen36moe, qwen38 bleiben FAIL | `synth/a5-double-score.sh`, Tabelle in F8; Referenz besteht V2 |
| P6 | U2 (12) | vermutlich 0 | nicht vorab prüfbar; generische Implementierungen sind nicht tool-spezifisch |
| P7, P8 | keine | — | reine Hinweise/Warnungen |

Jede Abweichung von „0" außerhalb P5 wäre ein echter Fund (ein Modell hätte eine Lücke benutzt) und ist einzeln
in `failure-analysis.md` zu dokumentieren, bevor korrigiert wird.

### 4.3 Entscheidung zu A5 (P5)

Zwei vertretbare Wege; die Vorabprüfung liefert die Zahlen für beide:

- **Option A — Vertragslesart (empfohlen):** V2 installieren, 8 Einträge in place auf PASS setzen, Annotation
  „unter defensiver Lesart FAIL (nur Property c)". Begründung: 8/8 Cloud-/Frontier-Modelle lesen den Brief
  identisch, der reale Aufrufer liefert sortiert, die Referenz besteht V2, und muse/codernext/Qwen fallen auch
  unter dieser Lesart aus fachlichen Gründen durch — A5 trennt dann weiterhin. Die Aberkennung des muse-PASS
  bleibt unter beiden Lesarten richtig.
- **Option B — defensive Lesart:** Brief um den Satz zur beliebigen Eingabeordnung ergänzen, (c) streng lassen,
  (b) und (e2) trotzdem übernehmen (0 Flips), A5 im Bericht als „von niemandem bestanden — Spec-Ambiguität"
  führen und die Vertragslesart als Zweitspalte im Dashboard zeigen. Nachteil: bisherige Läufe sahen den alten
  Brief; künftige Läufe wären nicht vergleichbar.

In beiden Fällen die Doppel-Wertung ins Dashboard (`run-annotations.json`) aufnehmen, damit die 8 Fälle nicht
als Modellschwäche gelesen werden.

### 4.4 In-Place-Korrektur-Politik

1. Vor dem ersten Regrade: `cp results.jsonl results.jsonl.bak-audit-$(date +%Y%m%d-%H%M)`.
2. Bewertet wird der **letzte** Eintrag je (Label, Task) mit dem installierten, gehärteten Grader und
   **absolutem** Workspace-Pfad (`runs/<label>/<task>/ws`).
3. Ändert sich das Urteil, wird **nur das `grade`-Feld dieser Zeile** ersetzt (wie beim muse-Regrade vom 24.08.);
   `seconds`, `exit`, `changed` bleiben, es wird keine neue Zeile angehängt, ältere überholte Zeilen (z. B. die
   oc-gemini37f-Infra-Fails) bleiben unangetastet.
4. Jeder Flip bekommt einen Nachtrag in `failure-analysis.md` (Label, Task, alt → neu, Patch-ID, Grund) und einen
   Eintrag in `dashboard/registry/run-annotations.json`; bei A5 zusätzlich die Zweitlesart.
5. Regrades mit unverändertem Urteil werden nicht in `results.jsonl` vermerkt, sondern nur als Log
   (`audit-scratch/regrade-<stamp>.log`) abgelegt — das ist der Nachweis, dass die Härtung nichts an ehrlichen
   Abgaben verschiebt.
6. Ab dem Regrade gilt: Grader-Änderungen sind Suite-Versionen; künftige Änderungen an `grade.sh`/`grade_test.go`
   lösen denselben Zyklus (Backup, Regrade aller letzten Einträge, In-Place-Korrektur, Nachtrag) aus.
7. Die Kopien der Abgaben unter `audit-scratch/` (`a5-*`, `a5aud-*`, `a6-attacker/legit-*`, `synth/a5-runs/*`)
   sind Arbeitskopien und keine Bewertungsgrundlage; bewertet wird ausschließlich `runs/`.

### Anhang — Artefakte des Nachlaufs (alle unter `audit-scratch/`)

`poc-0/` (A5 Kaperung, 4 Varianten + Kontrolle) · `poc-1/` (A5 chmod) · `poc-2/` (A6 ID-Sort, Reverse, Kontrolle)
· `poc-3/` (A6 chmod, `reproduce.sh`) · `a4/x*`, `a4/grade-fixed.sh` · `a6-attacker/e*`, `grade_fixed.sh`,
`grade_test_patched.go`, `inithang/` · `a5aud-*` · `a2a3/{a2-notwired,a2-inject,a2-enum,a3-hijack}` ·
`synth/{a1-hijack,a1-nobase,a3-chmod,a4-optinj,u1-inject,u2-chmod}` · `synth/a5-grade-v1_test.go`,
`a5-grade-v2_test.go`, `a5-grade-variant.sh`, `a5-double-score.sh`, `a5-runs/` · `synth/precheck-kept-tests.sh`.

# Sicherheitsmodell

**Frage dieser Seite: Warum kann ein Benchmark-Agent hier nichts kaputtmachen — und warum installiert sich nichts von selbst?**

## Sandbox: Der Agent sieht nur /work

Jeder Suite-Lauf startet einen rootless-podman-Container (`--userns=keep-id`, `--pull=never`). Gemountet sind genau:

- `/work` — eine **Wegwerf-Kopie** des Task-Workspace (`bench/runs/<label>/<task>/ws`)
- der Go-Modulcache, **read-only**
- die OpenCode-Config des Laufs

Kein `$HOME`, keine SSH-Keys, keine Secrets. Netz gibt es nur über pasta; der Host-Loopback ist als `169.254.1.2` gemappt (nur dafür, dass der Agent den Modell-Server erreicht). In der Bench-Config ist `webfetch` auf `deny`. Ein `timeout` (Default 1200 s, dann SIGTERM, nach 30 s SIGKILL) beendet hängende Läufe hart.

## Keine Remotes: Es gibt nichts zu pushen

Die Workspaces sind Git-Klone **ohne `origin`, ohne Hooks**; `deploy/`-Skripte, `.claude/` und `.github/` sind vor dem Lauf entfernt (`prepare-workspaces.sh`). Der Agent kann nichts pushen, nichts deployen, keine CI triggern. Die Original-Repos unter `~/Projects` werden von der gesamten Suite nie angefasst.

## Grader host-seitig: Der Agent benotet sich nicht selbst

`grade.sh` läuft **nach** Container-Ende auf dem Host. Der Agent sieht weder den Grader noch die versteckten Grading-Tests (die kopiert der Grader erst zur Bewertung in den Workspace). Zusätzlich prüft jeder Grader, dass Testdateien unverändert sind — `git diff` gegen den Baseline-Commit; ein einziger Byte Änderung heißt `FAIL test-file-modified`. `results.jsonl` beschreibt ausschließlich der Host. Dass das nötig ist, hat das Wochenende gezeigt: zwei Läufe (35B, 80B) waren funktional korrekt und fielen trotzdem durch, weil sie Testdateien anfassten — siehe [Benchmarks verstehen](benchmarks-verstehen.md).

## GPU-Lock: Ein Lauf zur Zeit

GPU und Modell-Server gehören dem aktiven Lauf. Der Läufe-Tab des Dashboards startet deshalb **nie** zwei Benchmarks parallel („Es läuft bereits ein Benchmark — bitte warten"). Gleiche Regel manuell: `perf-bench.sh` nie parallel zum laufenden `llama-server` — gleiche GPU, die Zahlen wären Müll.

## Warum Backends und Images nicht auto-installiert werden

- **Container-Images:** `--pull=never` — es läuft nur, was lokal explizit gebaut wurde (`agent-bench`, `agent-bench-dsh`, `aider-bench`). Kein Lauf zieht sich still ein neues Image aus dem Netz. dsh ist im Image auf `0.1.1-rc.2` **gepinnt** (Developer Preview — ein ungeplantes Update ändert Flags und Profile).
- **Runtime/Backends:** `llama-cpp` + `ggml-vulkan`/`ggml-hip`/`ggml-cpu` kommen aus pacman und werden bewusst per `sudo pacman -Syu` gepflegt. Neue Modell-Architekturen brauchen oft ein frisches llama.cpp — erst Runtime updaten, dann das GGUF laden, sonst gibt es Ladefehler oder **stillen Unsinn**. Ein Auto-Update mitten in einer Benchmark-Kampagne würde außerdem die Vergleichbarkeit der Zahlen zerstören.
- **Modelle:** GGUFs aktualisieren sich nicht selbst; neue Dateien landen manuell in `models/` plus `serve.sh`-Eintrag ([Anleitung](modell-hinzufuegen.md)).

## API-Keys

Keys nur als Umgebungsvariable exportieren oder in `~/ai-lab/.env` (steht in `.gitignore`). Nie in Skripte, Configs oder Docs. Die Runner reichen genau die benötigten Variablen in den Container durch (`-e DEEPSEEK_API_KEY …`) — sie stehen in keinem Image und keinem Log.

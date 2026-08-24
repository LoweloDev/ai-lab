#!/usr/bin/env python3
"""Wertet raw/<label>.* der A6-Robustheits-Batterie aus.

  summarize.py --result <label>   eine RESULT-Zeile + logs/<label>.log schreiben
  summarize.py --json             Gesamtzusammenfassung als JSON
  summarize.py --markdown         Tabelle + gerissene Tests je Label als Markdown-Fragment

Die Testliste wird aus battery_test.go gelesen (func TestZZBatReal*/TestZZBatPath*), damit
Batterie und Auswertung nicht auseinanderlaufen koennen.
"""
import glob
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
RAW = os.path.join(HERE, "raw")
LOGDIR = os.path.join(HERE, "logs")
BATTERY = os.path.join(HERE, "battery_test.go")


def read(path):
    try:
        with open(path, encoding="utf-8", errors="replace") as fh:
            return fh.read()
    except OSError:
        return ""


def test_names():
    names = re.findall(r"^func (TestZZBat\w+)\(", read(BATTERY), flags=re.M)
    real = [n for n in names if n.startswith("TestZZBatReal")]
    path = [n for n in names if n.startswith("TestZZBatPath")]
    return real, path


def evaluate(label):
    real, path = test_names()
    known = set(real + path)
    rc = read(os.path.join(RAW, f"{label}.rc")).strip()
    basebuild = read(os.path.join(RAW, f"{label}.basebuild"))
    stderr = read(os.path.join(RAW, f"{label}.stderr"))
    wsclean = read(os.path.join(RAW, f"{label}.wsclean")).strip()
    entry = {"label": label, "rc": rc, "wsclean": wsclean, "base_ok": "base_rc=0" in basebuild}

    if rc in ("IN-USE", "NO-FEED", "COLLISION"):
        entry["verdict"] = {"IN-USE": "uebersprungen (in Benutzung)", "NO-FEED": "nicht baubar (kein feed-Paket)",
                            "COLLISION": "uebersprungen (Kollision)"}[rc]
        return entry

    results = {}
    output = {}
    build_output = []
    build_failed = False
    log_lines = []
    for line in read(os.path.join(RAW, f"{label}.json")).splitlines():
        try:
            ev = json.loads(line)
        except ValueError:
            log_lines.append(line)
            continue
        action = ev.get("Action")
        if action in ("build-fail",):
            build_failed = True
        if action == "build-output":
            build_output.append(ev.get("Output", ""))
            continue
        name = ev.get("Test")
        if action == "output":
            text = ev.get("Output", "")
            log_lines.append(text.rstrip("\n"))
            if name in known:
                output.setdefault(name, []).append(text)
            continue
        if name in known and action in ("pass", "fail", "skip"):
            results[name] = action

    all_text = stderr + "".join(build_output) + "\n".join(log_lines)
    if "[build failed]" in all_text or "build failed" in stderr:
        build_failed = True
    if not results and rc != "0":
        build_failed = True

    # Lesbares Log
    os.makedirs(LOGDIR, exist_ok=True)
    with open(os.path.join(LOGDIR, f"{label}.log"), "w", encoding="utf-8") as fh:
        fh.write(f"# {label}: rc={rc} wsclean={wsclean} base_ok={entry['base_ok']}\n")
        if build_output:
            fh.write("".join(build_output))
        fh.write("\n".join(log_lines))
        fh.write("\n")
        if stderr.strip():
            fh.write("--- stderr:\n" + stderr)

    if build_failed or not results:
        entry["verdict"] = "nicht baubar"
        diag = [l for l in all_text.splitlines() if re.search(r"undefined:|cannot|syntax error|build failed|redeclared|not used", l)]
        entry["diagnose"] = "\n".join(diag[:8])
        return entry

    def bucket(names):
        passed = [n for n in names if results.get(n) == "pass"]
        failed = [n for n in names if results.get(n) == "fail"]
        missing = [n for n in names if n not in results]
        return passed, failed, missing

    rp, rf, rm = bucket(real)
    pp, pf, pm = bucket(path)
    entry.update(verdict="gelaufen",
                 real_pass=len(rp), real_total=len(real), real_failed=rf, real_missing=rm,
                 path_pass=len(pp), path_total=len(path), path_failed=pf, path_missing=pm)
    snippets = {}
    for name in rf + pf:
        lines = [l.strip() for l in "".join(output.get(name, [])).splitlines()
                 if l.strip() and not l.startswith("=== RUN") and not l.startswith("--- FAIL")]
        snippets[name] = lines[0] if lines else ""
    entry["fail_snippets"] = snippets
    return entry


def result_line(entry):
    label = entry["label"]
    guard = {"clean": "workspace-unveraendert", "DIRTY": "WORKSPACE-VERAENDERT"}.get(entry.get("wsclean"), "n/a")
    if entry["verdict"] != "gelaufen":
        return f"RESULT {label} status={entry['verdict']} rc={entry['rc']} base_ok={entry.get('base_ok')} guard={guard}"
    failed = ",".join(entry["real_failed"] + entry["path_failed"]) or "-"
    missing = entry["real_missing"] + entry["path_missing"]
    extra = f" missing=[{','.join(missing)}]" if missing else ""
    return (f"RESULT {label} status=gelaufen real={entry['real_pass']}/{entry['real_total']} "
            f"path={entry['path_pass']}/{entry['path_total']} failed=[{failed}]{extra} rc={entry['rc']} guard={guard}")


def all_labels():
    return sorted(os.path.basename(p)[:-3] for p in glob.glob(os.path.join(RAW, "*.rc")))


def markdown(entries):
    out = ["| Label | Real x/y | Path x/y | Bemerkung |", "|---|---|---|---|"]
    for e in entries:
        if e["verdict"] != "gelaufen":
            out.append(f"| {e['label']} | {e['verdict']} | — | {e.get('diagnose', '').splitlines()[0] if e.get('diagnose') else ''} |")
            continue
        failed = e["real_failed"] + e["path_failed"]
        note = "keine Risse" if not failed else "reisst: " + ", ".join(n.replace("TestZZBat", "") for n in failed)
        out.append(f"| {e['label']} | {e['real_pass']}/{e['real_total']} | {e['path_pass']}/{e['path_total']} | {note} |")
    out.append("")
    for e in entries:
        if e["verdict"] != "gelaufen" or not (e["real_failed"] + e["path_failed"]):
            continue
        out.append(f"### {e['label']} (Real {e['real_pass']}/{e['real_total']}, Path {e['path_pass']}/{e['path_total']})")
        for name in e["real_failed"] + e["path_failed"]:
            out.append(f"- `{name}` — {e['fail_snippets'].get(name, '')}")
        out.append("")
    return "\n".join(out)


def main(argv):
    if len(argv) >= 2 and argv[0] == "--result":
        entry = evaluate(argv[1])
        print(result_line(entry))
        return
    entries = [evaluate(label) for label in all_labels()]
    if argv and argv[0] == "--markdown":
        print(markdown(entries))
    else:
        print(json.dumps(entries, indent=1, ensure_ascii=False))


if __name__ == "__main__":
    main(sys.argv[1:])

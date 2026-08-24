#!/usr/bin/env python3
"""Wertet raw/*.json der A5-Robustheits-Batterie aus und druckt eine JSON-Zusammenfassung."""
import glob
import json
import os

RAW = "/home/lowelodev/ai-lab/bench/robustness-battery/a5/raw"

REAL = [
    "TestZZBatRealScoreTiesDesktopConserved",
    "TestZZBatRealScoreTiesMobileLeadsWithTopScore",
    "TestZZBatRealEmptyFeedNoBatches",
    "TestZZBatRealSingleTopicPool",
    "TestZZBatRealAllLivePool",
    "TestZZBatRealLargePool",
    "TestZZBatRealPageSizeBounds",
]
PATH = [
    "TestZZBatPathNaNScores",
    "TestZZBatPathNegativeAndInfiniteScores",
    "TestZZBatPathDuplicateIDs",
    "TestZZBatPathPageSizeZero",
    "TestZZBatPathPageSizeNegative",
]
ALL = REAL + PATH


def read(path):
    try:
        with open(path, encoding="utf-8", errors="replace") as fh:
            return fh.read()
    except OSError:
        return ""


def main():
    summary = {}
    for rcfile in sorted(glob.glob(os.path.join(RAW, "*.rc"))):
        label = os.path.basename(rcfile)[:-3]
        rc = read(rcfile).strip()
        basebuild = read(os.path.join(RAW, f"{label}.basebuild"))
        stderr = read(os.path.join(RAW, f"{label}.stderr"))
        wsclean = read(os.path.join(RAW, f"{label}.wsclean")).strip()

        results = {}       # testname -> pass/fail/skip
        fail_output = {}   # testname -> collected output lines
        build_failed = False
        for line in read(os.path.join(RAW, f"{label}.json")).splitlines():
            try:
                ev = json.loads(line)
            except ValueError:
                continue
            if ev.get("Action") in ("build-fail",):
                build_failed = True
            name = ev.get("Test")
            if not name or "/" in name or name not in ALL:
                continue
            action = ev.get("Action")
            if action in ("pass", "fail", "skip"):
                results[name] = action
            elif action == "output":
                fail_output.setdefault(name, []).append(ev.get("Output", ""))

        if "FAIL" in stderr and ("build failed" in stderr or "cannot find" in stderr or " undefined:" in stderr):
            build_failed = True
        if not results and rc not in ("0",):
            build_failed = True

        entry = {"rc": rc, "wsclean": wsclean, "base_ok": "base_rc=0" in basebuild}
        if build_failed or not results:
            entry["verdict"] = "nicht baubar"
            entry["stderr_head"] = "\n".join(stderr.splitlines()[:12])
            summary[label] = entry
            continue

        def bucket(names):
            passed = [n for n in names if results.get(n) == "pass"]
            failed = [n for n in names if results.get(n) == "fail"]
            missing = [n for n in names if n not in results]
            return passed, failed, missing

        rp, rf, rm = bucket(REAL)
        pp, pf, pm = bucket(PATH)
        entry.update(
            real_pass=len(rp), real_total=len(REAL), real_failed=rf, real_missing=rm,
            path_pass=len(pp), path_total=len(PATH), path_failed=pf, path_missing=pm,
            verdict="gelaufen",
        )
        entry["fail_snippets"] = {
            n: "".join(fail_output.get(n, []))[-1200:] for n in rf + pf
        }
        summary[label] = entry

    print(json.dumps(summary, indent=1, ensure_ascii=False))


if __name__ == "__main__":
    main()

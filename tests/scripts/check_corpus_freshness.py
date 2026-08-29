#!/usr/bin/env python3
"""Check Go's cases.json against the JS SDK's cases.json.

The two corpora are maintained by hand; `specVersion` is a label, not a
contract. This script diffs the corpora and makes drift visible.

Diff categories:

  - "missing" — JS has a case name Go doesn't.
                Fails CI unless the name is in skiplist["missing"][key].
  - "drift"   — Both sides have the case name, but the bodies differ
                (canonical-JSON SHA1 mismatch). Fails CI unless the name
                is in skiplist["drift"][key]. Catches the silent
                case-body update that pure name-matching misses.
  - "extra"   — Go has a case name JS doesn't. Reported as
                informational only — Go carries documented extensions
                plus locally-authored regressions. Never fails CI.

Source-of-truth URL is configurable via --js-source or env GB_JS_CASES_URL.
Defaults to the JS SDK's main-branch raw URL.

Exit codes:
  0 — no actionable findings (or all on skiplist), OR the JS source could
      not be fetched (network blip) — that is a warning, not a failure, so a
      transient outage doesn't break unrelated builds; pass --strict-fetch
      to make a failed fetch exit 2 instead (for scheduled audit runs,
      where a silently skipped check is a missed alert)
  1 — at least one missing or drifted case isn't on the skiplist
  2 — local IO/parse error (missing cases.json or bad skiplist), or a
      failed fetch under --strict-fetch

Run locally:
  python3 tests/scripts/check_corpus_freshness.py
  python3 tests/scripts/check_corpus_freshness.py --js-source /path/to/local/cases.json
  GB_JS_CASES_URL=https://... python3 tests/scripts/check_corpus_freshness.py
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import urllib.error
import urllib.request
from collections import Counter
from pathlib import Path
from typing import Dict, List, Set, Tuple

REPO_ROOT = Path(__file__).resolve().parents[2]
LOCAL_CASES = REPO_ROOT / "cases.json"
SKIPLIST = REPO_ROOT / "tests" / "scripts" / "corpus_skiplist.json"

DEFAULT_JS_URL = "https://raw.githubusercontent.com/growthbook/growthbook/main/packages/sdk-js/test/cases.json"

# Top-level keys to diff. Other keys in cases.json (specVersion, decrypt
# binary blobs, urlRedirect which Go doesn't yet wire) are skipped either
# because they're scalar metadata or because the divergence is tracked
# separately.
KEYS_TO_DIFF = (
    "evalCondition",
    "feature",
    "run",
    "hash",
    "getBucketRange",
    "chooseVariation",
    "getQueryStringOverride",
    "inNamespace",
    "getEqualWeights",
    "stickyBucket",
    "contextualBandit",
)


def _fetch_js_cases(source: str) -> dict:
    """Fetch JS cases.json from a URL or local path."""
    if source.startswith(("http://", "https://")):
        try:
            req = urllib.request.Request(source, headers={"User-Agent": "growthbook-golang-corpus-check"})
            with urllib.request.urlopen(req, timeout=20) as resp:  # noqa: S310
                return json.loads(resp.read().decode("utf-8"))
        except urllib.error.URLError as e:
            raise RuntimeError(f"fetch failed: {source}: {e}") from e
        except json.JSONDecodeError as e:
            raise RuntimeError(f"JS source did not return valid JSON: {e}") from e
    path = Path(source)
    if not path.is_file():
        raise RuntimeError(f"local source not found: {source}")
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as e:
        raise RuntimeError(f"local source invalid JSON: {e}") from e


def _load_local_cases() -> dict:
    if not LOCAL_CASES.is_file():
        raise RuntimeError(f"local cases.json not found: {LOCAL_CASES}")
    return json.loads(LOCAL_CASES.read_text(encoding="utf-8"))


def _load_skiplist() -> Dict[str, Dict[str, Set[str]]]:
    """Load skiplist. File format:

        {
          "missing": { "<top_level_key>": ["case name", ...] },
          "drift":   { "<top_level_key>": ["case name", ...] }
        }

    `missing` — case names JS has and Go deliberately doesn't carry yet.
    `drift`   — case names where Go deliberately keeps a different body.

    Extras (Go has, JS doesn't) are reported but never fail. The file is
    optional.
    """
    if not SKIPLIST.is_file():
        return {"missing": {}, "drift": {}}
    try:
        data = json.loads(SKIPLIST.read_text(encoding="utf-8"))
    except json.JSONDecodeError as e:
        raise RuntimeError(f"skiplist invalid JSON: {e}") from e
    return {
        "missing": {k: set(v) for k, v in (data.get("missing") or {}).items()},
        "drift": {k: set(v) for k, v in (data.get("drift") or {}).items()},
    }


def _case_signatures_grouped(cases: list) -> Dict[str, List[str]]:
    """Return {case_name: [body_hash, ...]} preserving order.

    Body = everything after the name (case[1:]), serialized via canonical
    JSON (sorted keys + compact separators) so logically-equal cases hash
    the same regardless of key order or whitespace. Same-named cases keep
    every occurrence so drift in any duplicate is visible.
    """
    out: Dict[str, List[str]] = {}
    for c in cases:
        if not (isinstance(c, list) and c and isinstance(c[0], str)):
            continue
        name = c[0]
        body = json.dumps(c[1:], sort_keys=True, separators=(",", ":"))
        h = hashlib.sha1(body.encode("utf-8")).hexdigest()[:16]
        out.setdefault(name, []).append(h)
    return out


def _diff(
    js_cases: dict, local_cases: dict, skip: Dict[str, Set[str]]
) -> Tuple[Dict[str, List[str]], Dict[str, List[str]], Dict[str, List[str]], Dict[str, List[str]], Dict[str, List[str]]]:
    actionable_missing: Dict[str, List[str]] = {}
    skipped_missing: Dict[str, List[str]] = {}
    extras: Dict[str, List[str]] = {}
    actionable_drift: Dict[str, List[str]] = {}
    skipped_drift: Dict[str, List[str]] = {}

    missing_skip = skip.get("missing", {})
    drift_skip = skip.get("drift", {})

    for key in KEYS_TO_DIFF:
        js_list = js_cases.get(key, [])
        local_list = local_cases.get(key, [])
        if not isinstance(js_list, list) or not isinstance(local_list, list):
            continue

        js_grouped = _case_signatures_grouped(js_list)
        local_grouped = _case_signatures_grouped(local_list)

        js_names_ordered: List[str] = []
        seen_js: Set[str] = set()
        for c in js_list:
            if isinstance(c, list) and c and isinstance(c[0], str) and c[0] not in seen_js:
                js_names_ordered.append(c[0])
                seen_js.add(c[0])

        local_names_ordered: List[str] = []
        seen_local: Set[str] = set()
        for c in local_list:
            if isinstance(c, list) and c and isinstance(c[0], str) and c[0] not in seen_local:
                local_names_ordered.append(c[0])
                seen_local.add(c[0])

        missing = [n for n in js_names_ordered if n not in local_grouped]
        extra = [n for n in local_names_ordered if n not in js_grouped]
        drift = [n for n in js_names_ordered if n in local_grouped and Counter(js_grouped[n]) != Counter(local_grouped[n])]

        key_missing_skip = missing_skip.get(key, set())
        key_drift_skip = drift_skip.get(key, set())

        actionable_missing[key] = [n for n in missing if n not in key_missing_skip]
        skipped_missing[key] = [n for n in missing if n in key_missing_skip]
        extras[key] = extra
        actionable_drift[key] = [n for n in drift if n not in key_drift_skip]
        skipped_drift[key] = [n for n in drift if n in key_drift_skip]

    return actionable_missing, skipped_missing, extras, actionable_drift, skipped_drift


def _spec_versions(js_cases: dict, local_cases: dict) -> Tuple[str, str]:
    return (str(js_cases.get("specVersion", "<unset>")), str(local_cases.get("specVersion", "<unset>")))


def _format_report(
    js_spec: str,
    local_spec: str,
    actionable_missing: Dict[str, List[str]],
    skipped_missing: Dict[str, List[str]],
    extras: Dict[str, List[str]],
    actionable_drift: Dict[str, List[str]],
    skipped_drift: Dict[str, List[str]],
) -> str:
    lines = []
    lines.append("=== Corpus freshness check (Go vs JS SDK) ===")
    lines.append(f"  JS specVersion: {js_spec}")
    lines.append(f"  Go specVersion: {local_spec}")
    if js_spec != local_spec:
        lines.append("  ⚠ specVersion mismatch — bump Go's value when you catch up to JS's.")
    lines.append("")

    n_missing = sum(len(v) for v in actionable_missing.values())
    n_skip_missing = sum(len(v) for v in skipped_missing.values())
    n_drift = sum(len(v) for v in actionable_drift.values())
    n_skip_drift = sum(len(v) for v in skipped_drift.values())
    n_extra = sum(len(v) for v in extras.values())

    if n_missing + n_drift == 0:
        lines.append(f"OK: no missing/drifted cases (skipped-missing: {n_skip_missing}, skipped-drift: {n_skip_drift}, extras: {n_extra})")
    else:
        lines.append(f"DRIFT: {n_missing} missing + {n_drift} body-drift (skipped: {n_skip_missing} missing, {n_skip_drift} drift; {n_extra} Go extras)")
    lines.append("")

    def _section(title: str, data: Dict[str, List[str]]) -> None:
        if not any(data.values()):
            return
        lines.append(f"--- {title} ---")
        for key, names in data.items():
            if not names:
                continue
            lines.append(f"  [{key}] ({len(names)})")
            for n in names:
                lines.append(f"    - {n}")
        lines.append("")

    _section("Missing in Go (FAILS CI)", actionable_missing)
    _section("Body-drift: same name, different case body (FAILS CI)", actionable_drift)
    _section("Missing in Go — skipped via corpus_skiplist.json", skipped_missing)
    _section("Body-drift — skipped via corpus_skiplist.json", skipped_drift)
    _section("Extra in Go (informational; never fails)", extras)
    return "\n".join(lines)


def main(argv: List[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument(
        "--js-source",
        default=os.environ.get("GB_JS_CASES_URL", DEFAULT_JS_URL),
        help="URL or local path to JS cases.json (default: JS SDK main branch)",
    )
    parser.add_argument("--json", action="store_true", help="output machine-readable JSON instead of text")
    parser.add_argument(
        "--strict-fetch",
        action="store_true",
        help="treat a failed fetch of the JS source as an error (exit 2) instead of warn-and-pass",
    )
    args = parser.parse_args(argv)

    # Local corpus / skiplist problems are real repo errors → hard fail.
    try:
        local_cases = _load_local_cases()
        skip = _load_skiplist()
    except RuntimeError as e:
        print(f"corpus check infra error: {e}", file=sys.stderr)
        return 2

    # A failure to fetch the JS source (network blip, GitHub outage) must not
    # break a PR build — the check is advisory drift detection, not a gate on
    # the SDK itself. Warn and pass, unless --strict-fetch (scheduled audit
    # runs, where a silently skipped check is a missed alert).
    try:
        js_cases = _fetch_js_cases(args.js_source)
    except RuntimeError as e:
        if args.strict_fetch:
            print(f"corpus check fetch error: {e}", file=sys.stderr)
            return 2
        print(f"WARNING: corpus freshness check skipped — could not fetch JS cases: {e}", file=sys.stderr)
        return 0

    actionable_missing, skipped_missing, extras, actionable_drift, skipped_drift = _diff(js_cases, local_cases, skip)
    js_spec, local_spec = _spec_versions(js_cases, local_cases)

    if args.json:
        print(
            json.dumps(
                {
                    "js_specVersion": js_spec,
                    "go_specVersion": local_spec,
                    "missing_actionable": actionable_missing,
                    "missing_skipped": skipped_missing,
                    "drift_actionable": actionable_drift,
                    "drift_skipped": skipped_drift,
                    "extras": extras,
                },
                indent=2,
                sort_keys=True,
            )
        )
    else:
        print(_format_report(js_spec, local_spec, actionable_missing, skipped_missing, extras, actionable_drift, skipped_drift))

    fail = any(actionable_missing.values()) or any(actionable_drift.values())
    return 1 if fail else 0


if __name__ == "__main__":
    sys.exit(main())

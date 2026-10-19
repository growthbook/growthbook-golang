"""Unit tests for the existing freshness checker, not a second corpus check.

check_corpus_freshness.py compares all configured suites, including the nested
savedGroupReferencesV2 suites. These tests deliberately remove or change cases
to verify that the checker catches them; comparing two matching corpora alone
would not catch a checker that accidentally skipped a suite. Keeping these
unit tests separate avoids mixing test fixtures into the operational checker.
They use synthetic data only, with no network access or repository changes.

Run from the repository root when changing the checker or its suite list:
    python3 -m unittest discover -s tests/scripts -p 'test_*.py'

These tests are not run by `go test` or the current CI workflow. The separate
corpus-freshness workflow runs check_corpus_freshness.py on relevant PRs against
the pinned JS corpus, and weekly/on manual dispatch against live JS main.
"""

import copy
import unittest

from check_corpus_freshness import _diff


class SavedGroupCorpusTests(unittest.TestCase):
    def test_nested_suites_are_checked(self):
        for suite in ("evalCondition", "feature", "run"):
            with self.subTest(suite=suite):
                upstream = {"savedGroupReferencesV2": {suite: [["case", True]]}}
                key = "savedGroupReferencesV2." + suite
                missing, _, _, drift, _ = _diff(upstream, {}, {})
                self.assertEqual(missing[key], ["case"])
                self.assertEqual(drift[key], [])

                changed = copy.deepcopy(upstream)
                changed["savedGroupReferencesV2"][suite][0][1] = False
                missing, _, _, drift, _ = _diff(upstream, changed, {})
                self.assertEqual(missing[key], [])
                self.assertEqual(drift[key], ["case"])

                missing, _, _, drift, _ = _diff(upstream, upstream, {})
                self.assertEqual(missing[key], [])
                self.assertEqual(drift[key], [])


if __name__ == "__main__":
    unittest.main()

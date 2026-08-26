# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

- Fixed a data race on the sticky bucket assignments cache: it is shared by
  reference between a client and its clones and was mutated during evaluation
  without synchronization. The cache is now mutex-guarded, and assignment
  documents are no longer mutated in place once stored.
- Concurrent sticky bucket saves for the same user now merge instead of
  overwriting each other (saves are serialized per document key). The
  guarantee is scoped to a client and its clones: independently constructed
  clients or separate processes sharing one `StickyBucketService` can still
  race at the service, whose `SaveAssignments` is last-write-wins.
- Cache-miss reads of sticky bucket assignment docs run under the same
  per-document lock as saves, so a read racing a save can no longer
  re-install a stale doc after the save's fresher one was evicted from the
  bounded cache.
- The sticky bucket assignments cache is now bounded (LRU, 10000 entries by
  default) instead of growing with every attribute value evaluated. New
  `WithStickyBucketCacheSize` client option tunes or removes the bound.
- Direct (no-operator) boolean condition comparison now matches the JS SDK
  exactly: a condition like `{"userId": false}` no longer matches a missing or
  null attribute (JS: `value !== null && !!value === condition`). String and
  number conditions keep JS's coercive comparison (`value + ""`, `value * 1`).
  **Compatibility note:** no API changes, but boolean conditions evaluated
  against missing/null attributes now return false instead of matching `false`.
- Synced `cases.json` with the JS SDK conformance corpus (0.8.0 bodies; spec
  label 0.7.1 — the four contextual-bandit cases are skiplisted as CB rules are
  not implemented).
- Added a corpus-freshness CI check against the JS SDK cases corpus to catch
  missing or drifted cases (`tests/scripts/check_corpus_freshness.py`).

## [v0.2.1](https://pkg.go.dev/github.com/growthbook/growthbook-golang@v0.2.1) - 2025-01-25

- fix WithAttributeOverrides panic when applied to nil attributes

## [v0.2.0](https://pkg.go.dev/github.com/growthbook/growthbook-golang@v0.2.0) - 2025-01-25

- Major refactoring of the SDK to address concurrency issues, improving thread safety.
- Introduced feature options for client configuration.
- Separated shared and local data, enabling the creation of child client instances.
- Removed the custom GrowthBook context and adopted Go's native context for API calls.
- Switched to native JSON unmarshaling for better performance and compatibility.
- Extracted an internal value package for representing conditions and attribute values more robustly.
- Extracted an internal condition package with a more type-safe approach to condition representation.
- Updated spec.json to the latest 0.7.0 version and adopted a type-safe approach for parsing specs.
- Synchronized internal structures with the current state of the JavaScript SDK.
- Implemented background data sources for feature loading via polling and SSE streaming.

## [v0.1.4](https://pkg.go.dev/github.com/growthbook/growthbook-golang@v0.1.4) - 2023-08-05

- Fix numeric comparisons to use Javascript semantics.
- Provide access to last feature update time.
- Get to parity with JS SDK spec version v0.4.2.
- Implement and use new hash function.
- Add ranges, filters, URL targeting to feature rules.
- Add version comparison operators.
- Implement feature repository.
- Add encrypted features.
- CI improvements: Go version matrix tests.


## [v0.1.3](https://pkg.go.dev/github.com/growthbook/growthbook-golang@v0.1.3) - 2023-04-24

- Fix all JSON tests from v0.2.3 test spec.
- Add `UsedHash` and `FeatureID` fields in the `ExperimentResult` type.
- Improve logging for JSON tests.
- Handle hash value type variants.
- Update JSON cases to v0.2.3.


## [v0.1.2](https://pkg.go.dev/github.com/growthbook/growthbook-golang@v0.1.2) - 2023-04-24

- Allow arrays in attributes.
- Allow nil context in `GrowthBook` constructor.
    

## [v0.1.1](https://pkg.go.dev/github.com/growthbook/growthbook-golang@v0.1.1) - 2023-04-20

- Improve handling of array and slice values in attributes.
- Documentation improvements.
- CI setup.


## [v0.1.0](https://pkg.go.dev/github.com/growthbook/growthbook-golang@v0.1.0) - 2022-01-25

- Initial release.

# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

- Steady-state evaluations no longer call the sticky bucket service: when
  the client's assignment cache already holds the exact assignment for the
  primary hash attribute, the save short-circuits before taking the
  per-document lock and re-reading the service. Previously every
  in-experiment evaluation performed one `GetAssignments` round-trip under
  the doc lock just to discover nothing changed — JS and Python answer this
  from memory. Fallback-to-primary doc upgrades are unaffected (the check is
  against the primary doc only).
- Fixed: a negative experiment `bucketVersion` saved sticky assignments under
  a different key than reads looked up (`exp__-1` vs `exp__0`), so the saved
  assignment was never found again. Keys now normalize negative versions to
  0 in one place, for reads and saves alike.

## [v0.4.0](https://pkg.go.dev/github.com/growthbook/growthbook-golang@v0.4.0) - 2026-08-28

- **Breaking change:** `ExperimentCallback` gains a `*TrackingUserContext`
  parameter — the evaluating client's attributes and URL, the JS SDK
  `trackingCallback`'s user argument — before the extra-data parameter:
  `func(ctx, experiment, result, userCtx, extraData)`. Existing callbacks
  need the one added parameter to compile. The same user context is stamped
  on each buffered `TrackingData` as its `user` field, snapshotted at
  evaluation time. `EventUserContext` is now an alias of the new
  `TrackingUserContext` type; event logger code is unaffected.
- **Behavior change (JS parity):** `EvalFeature` now fires `ExperimentCallback`
  and plugin `OnExperimentViewed` for every experiment assignment made during
  evaluation, in evaluation order — including passthrough assignments (e.g.
  the control arm of a monitored ramp step) and assignments made while
  evaluating prerequisite features. Previously only the experiment that
  decided the served value was reported, so those exposures were silently
  dropped. Feature-usage callbacks fire before experiment callbacks. Unlike
  the JS SDK, callbacks are deduplicated per evaluation, not per client
  lifetime — repeat evaluations fire them again (the deferred tracking
  buffer dedupes for its lifetime, and analysis dedupes exposures at query
  time).
- **Behavior change (JS parity):** `FeatureUsageCallback` and plugin
  `OnFeatureEvaluated` now also fire for prerequisite features evaluated
  along the way, not just the requested feature. A feature is reported once
  per evaluation unless its value changed.
- **Behavior change (JS parity):** forced variations (`force`, forced
  variations set on the client, querystring overrides) no longer fire
  tracking callbacks — from `RunExperiment` or from a feature's experiment
  rules in `EvalFeature`. These are not randomized exposures and the JS SDK
  has never tracked them.
- Added deferred tracking, the Go equivalent of the JS SDK's deferred
  tracking queue, for servers that forward exposures to client SDKs (e.g.
  remote evaluation) instead of tracking via callbacks. Enable it with the
  `WithDeferredTracking` option — typically on a per-request child client,
  which acts as the user context — and every experiment exposure produced by
  the standard evaluation methods is buffered: read it with
  `Client.DeferredTrackingCalls` (which returns detached copies, safe to
  retain or mutate) and empty it with `Client.ClearDeferredTrackingCalls`.
  The buffer deduplicates by `TrackingData.DedupeKey` (the same fields as
  the JS SDK's dedupe key) over its lifetime and keeps first-seen order.
  `TrackingData` serializes to the
  JS SDK's `TrackingData` shape, compatible with `setDeferredTrackingCalls`.
  Callbacks and plugins are unaffected and keep firing per evaluation.
- **Behavior change:** `Namespace` and `BucketRange` now marshal to their
  payload tuple forms (`[id, start, end]` and `[min, max]`), matching what
  they unmarshal from and what other SDKs emit. Previously they marshaled as
  structs that could not be re-parsed, which broke JSON round-trips of
  experiments (including deferred-tracking deep copies) for any experiment
  carrying a namespace, ranges, or filters.

## [v0.3.0](https://pkg.go.dev/github.com/growthbook/growthbook-golang@v0.3.0) - 2026-08-26

- **Behavior change (JS parity):** attribute values that are falsy in JS
  (`0`, `false`, `""`, `null`) now count as missing when resolving hash
  attributes — affected users fall out of experiments and coverage rollouts
  (or fall through to the fallback attribute) instead of being bucketed.
  Applies to experiment assignment, rollout inclusion, and sticky bucket
  fallback lookups.
- **Behavior change (JS parity):** `urlPatterns`, `url`, `groups`, and
  `status` on feature rules are no longer applied during evaluation and are
  now deprecated — the JS SDK has never supported them on feature rules, and
  payloads served by the GrowthBook API never include them (support existed
  only in v0.2.9, for hand-built or custom feature payloads). Rules relying
  on `status: "draft"` as an off-switch will start serving. Experiment-level
  targeting via `RunExperiment` is unchanged.
- **Behavior change (JS parity):** `fallbackAttribute` (on rules and
  experiments) is only consulted when a sticky bucket service is configured
  and sticky bucketing isn't disabled, matching the JS SDK. Without a sticky
  bucket service, users missing the primary hash attribute are now excluded
  from those experiments instead of being bucketed by the fallback.
- Coverage rollouts on force rules now pass the rule's `fallbackAttribute`
  when sticky bucketing is active (previously never passed).
- Added `Client.LogEvent` for logging custom events — the Go equivalent of
  the JS SDK's `logEvent` — with the `WithEventLogger` option and an optional
  `EventLoggerPlugin` plugin interface. `GrowthBookTrackingPlugin` implements
  it, batching custom events to the ingestor alongside automatic events.
- Tracking events are marshaled individually at enqueue time, so one
  unserializable value drops only that event instead of a whole batch.
- `WithSseMaxRetryInterval(0)` or negative values keep the safe default
  backoff cap instead of reintroducing unbounded backoff, and
  `WithSseDataSource(nil)` is ignored safely.
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

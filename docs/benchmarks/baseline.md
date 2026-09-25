# SDK Benchmarks

A versioned log of `go test -bench=. -benchmem -run=^$` results. Append a new
section per PR that touches the eval path so we can diff numbers over time.

## How to capture

```sh
go test -bench=. -benchmem -run=^$ -benchtime=2s . | tee /tmp/bench.txt
```

Then paste the output below under a new dated heading, along with:

- commit SHA (`git rev-parse HEAD`)
- Go version (`go version`)
- machine / CPU
- short note on what the change was meant to affect

## Bench targets (in `evaluator_bench_test.go`)

- `BenchmarkEvalFeature_Cold` — 1 feature, 1 rule. Floor for `EvalFeature`.
- `BenchmarkEvalFeature_Warm` — 50 features × 5 rules. Realistic shape.
- `BenchmarkRunExperiment` — direct `RunExperiment` with a 4-variation experiment.
- `BenchmarkEvalFeature_Parallel` — `b.RunParallel` to surface lock contention.
- `BenchmarkIsURLTargeted_Simple` / `_Regex` — URL targeting cost per call.

---

## 2026-04-30 — Baseline (parity PR: URL targeting + groups + status + Subscribe)

- Commit: `56435e2c53aa73d271ea5caab8159e80c3011bf1` (branch `feat/sdk-parity-url-groups-subscribe`)
- Go: `go1.23.6 darwin/arm64`
- CPU: Apple M1 Max
- Note: First baseline. URL targeting added in this PR; eval-path otherwise unchanged.

```
goos: darwin
goarch: arm64
pkg: github.com/growthbook/growthbook-golang
cpu: Apple M1 Max
BenchmarkEvalFeature_Cold-10        	17775379	       126.1 ns/op	     160 B/op	       3 allocs/op
BenchmarkEvalFeature_Warm-10        	17356208	       137.1 ns/op	     160 B/op	       3 allocs/op
BenchmarkRunExperiment-10           	10057777	       237.9 ns/op	     280 B/op	       4 allocs/op
BenchmarkEvalFeature_Parallel-10    	 8622685	       278.6 ns/op	     160 B/op	       3 allocs/op
BenchmarkIsURLTargeted_Simple-10    	  289328	      8050 ns/op	   12139 B/op	     126 allocs/op
BenchmarkIsURLTargeted_Regex-10     	  486561	      4955 ns/op	    8211 B/op	      94 allocs/op
```

### Observations / candidates for follow-up PRs

- **`EvalFeature` is fast** (~126ns/op, 3 allocs). No urgent work here.
- **URL targeting is hot**: `isURLTargeted` allocates 100+ times and recompiles
  regexes on every call (`regexp.Compile` inside `evalSimpleURLPart` and
  `evalRegexURLTarget`). For experiments that use `urlPatterns`, this is paid
  per eval. Two obvious wins:
  1. Pre-compile patterns at `URLTarget` parse time (or memoize in a small
     concurrent cache keyed by pattern string).
  2. Use `sync.OnceValue` per `URLTarget` to compile lazily once.
  Worth ~50× speedup at the URL-targeting layer; not yet on the critical path
  for users without URL-targeted experiments, so deferring to a follow-up PR.
- **`RunExperiment` parallel** shows ~2× per-op time vs. sequential, suggesting
  some lock contention. Likely `data.mu` RLock in `client.evaluator()`. If
  parallel throughput becomes important, consider snapshotting under a single
  RLock and passing through.

---

## 2026-05-06 - Post-review parity fixes

- Commit: `5397795` (branch `feat/sdk-parity-url-groups-subscribe`)
- Go: `go1.23.6 darwin/arm64`
- CPU: Apple M1 Max
- Note: After review fixes for URL/status/subscription parity, legacy `url`,
  strict `$in`/`$nin`, deterministic condition operator ordering, and subscriber
  notification optimization when no listeners are registered.

```
goos: darwin
goarch: arm64
pkg: github.com/growthbook/growthbook-golang
cpu: Apple M1 Max
BenchmarkEvalFeature_Cold-10        	17892104	       129.2 ns/op	     160 B/op	       3 allocs/op
BenchmarkEvalFeature_Warm-10        	16930773	       138.5 ns/op	     160 B/op	       3 allocs/op
BenchmarkRunExperiment-10           	 9422572	       239.2 ns/op	     280 B/op	       4 allocs/op
BenchmarkEvalFeature_Parallel-10    	 8419987	       283.8 ns/op	     160 B/op	       3 allocs/op
BenchmarkIsURLTargeted_Simple-10    	  294308	      7881 ns/op	   12138 B/op	     126 allocs/op
BenchmarkIsURLTargeted_Regex-10     	  462918	      4962 ns/op	    8211 B/op	      94 allocs/op
```

### Observations

- `EvalFeature` remains close to the first baseline and keeps the same 3 allocs.
- `RunExperiment` is effectively back to the first baseline after avoiding
  subscriber work when no listeners exist.
- URL targeting remains the same hotspot as the first baseline.

---

## 2026-09-17 - Saved group references v2 and encrypted saved groups

- Base commit: `453db2f3b377abbc8f8d6e02c5bec0cd3f20948e`; after measurements
  include the uncommitted saved-group changes.
- Go: `go1.27.1 darwin/arm64`
- CPU: Apple M5 Pro
- Command: `go test -bench=. -benchmem -run=^$ -benchtime=2s .`
- Note: Thread required branch-local cycle state through condition evaluation, compile
  v2 groups at payload load, decrypt saved groups during updates, and guard
  missing keys in object comparison. These existing benchmarks measure the
  effect on evaluation without v2 references.

Before:

```text
BenchmarkEvalFeature_Cold-18                              31332271       78.22 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_Warm-18                              28481265       83.55 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_ExperimentCallback-18    40926152       59.98 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_FeatureUsageCallback-18   14469081       165.1 ns/op     808 B/op     6 allocs/op
BenchmarkRunExperiment-18                                 11960678       201.7 ns/op     856 B/op     5 allocs/op
BenchmarkEvalFeature_Parallel-18                          12136455       207.7 ns/op     256 B/op     3 allocs/op
BenchmarkIsURLTargeted_Simple-18                            597475        4015 ns/op   12079 B/op   123 allocs/op
BenchmarkIsURLTargeted_Regex-18                             911192        2533 ns/op    8183 B/op    93 allocs/op
```

After:

```text
BenchmarkEvalFeature_Cold-18                              31595667       74.10 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_Warm-18                              30383031       83.56 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_ExperimentCallback-18    37638447       61.63 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_FeatureUsageCallback-18   15523171       157.6 ns/op     808 B/op     6 allocs/op
BenchmarkRunExperiment-18                                 12538068       197.1 ns/op     856 B/op     5 allocs/op
BenchmarkEvalFeature_Parallel-18                          11649919       205.5 ns/op     256 B/op     3 allocs/op
BenchmarkIsURLTargeted_Simple-18                            577120        3948 ns/op   12089 B/op   123 allocs/op
BenchmarkIsURLTargeted_Regex-18                             981750        2496 ns/op    8183 B/op    93 allocs/op
```

No added allocations in these paths. Single-run timing differences are small
and should not be interpreted as a demonstrated speedup or regression.

---

## 2026-09-18 - Legacy operators reading v2 list groups

- Base commit: `40fa400e01bc7ac7935517b5af944223f1fea9d0`; after measurements
  include the uncommitted typed-list membership changes.
- Go: `go1.27.1 darwin/arm64`
- CPU: Apple M5 Pro
- Command: `go test -bench=. -benchmem -run=^$ -benchtime=2s .`
- Note: `$inGroup` and `$notInGroup` share `$in` membership evaluation for
  legacy arrays and typed lists. Typed-list membership is compiled at load time.
  These existing benchmarks cover general evaluation, not saved-group lookup.

Before:

```text
BenchmarkEvalFeature_Cold-18                              26250964       76.63 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_Warm-18                              29780076       82.24 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_ExperimentCallback-18    40538714       60.27 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_FeatureUsageCallback-18   15117300       160.6 ns/op     808 B/op     6 allocs/op
BenchmarkRunExperiment-18                                 12181900       198.4 ns/op     856 B/op     5 allocs/op
BenchmarkEvalFeature_Parallel-18                          11527704       207.7 ns/op     256 B/op     3 allocs/op
BenchmarkIsURLTargeted_Simple-18                            560497        4112 ns/op   12077 B/op   123 allocs/op
BenchmarkIsURLTargeted_Regex-18                             853286        2591 ns/op    8183 B/op    93 allocs/op
```

After:

```text
BenchmarkEvalFeature_Cold-18                              31403380       77.67 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_Warm-18                              29562642       81.55 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_ExperimentCallback-18    41084546       59.84 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_FeatureUsageCallback-18   14558306       161.1 ns/op     808 B/op     6 allocs/op
BenchmarkRunExperiment-18                                 12498462       196.5 ns/op     856 B/op     5 allocs/op
BenchmarkEvalFeature_Parallel-18                          11612832       206.4 ns/op     256 B/op     3 allocs/op
BenchmarkIsURLTargeted_Simple-18                            569654        3956 ns/op   12086 B/op   123 allocs/op
BenchmarkIsURLTargeted_Regex-18                             924614        2600 ns/op    8183 B/op    93 allocs/op
```

Allocation counts are unchanged. Single-run timings do not establish a speedup
or regression.

---

## 2026-09-23 - Object saved-group references and attribute overrides

- Base commit: `40fa400e01bc7ac7935517b5af944223f1fea9d0`; both measurements
  include the September 18 local changes. After also includes object references
  and per-reference attribute overrides.
- Go: `go1.27.1 darwin/arm64`
- CPU: Apple M5 Pro
- Command: `go test -bench=. -benchmem -run=^$ -benchtime=2s .`
- Note: Reference objects and override paths are parsed at load time. These
  existing benchmarks cover general evaluation, not saved-group lookup.

Before:

```text
BenchmarkEvalFeature_Cold-18                              31123442       76.79 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_Warm-18                              29028614       83.39 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_ExperimentCallback-18    40088416       59.91 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_FeatureUsageCallback-18   15093211       165.3 ns/op     808 B/op     6 allocs/op
BenchmarkRunExperiment-18                                 12362053       199.4 ns/op     856 B/op     5 allocs/op
BenchmarkEvalFeature_Parallel-18                          11852841       207.3 ns/op     256 B/op     3 allocs/op
BenchmarkIsURLTargeted_Simple-18                            576253        4017 ns/op   12086 B/op   123 allocs/op
BenchmarkIsURLTargeted_Regex-18                             830262        2577 ns/op    8183 B/op    93 allocs/op
```

After:

```text
BenchmarkEvalFeature_Cold-18                              31403156       76.84 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_Warm-18                              29363739       83.58 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_ExperimentCallback-18    39311502       67.36 ns/op     256 B/op     3 allocs/op
BenchmarkEvalFeature_ObjectValue_FeatureUsageCallback-18   15364158       161.7 ns/op     808 B/op     6 allocs/op
BenchmarkRunExperiment-18                                 12252484       197.6 ns/op     856 B/op     5 allocs/op
BenchmarkEvalFeature_Parallel-18                          11594487       206.2 ns/op     256 B/op     3 allocs/op
BenchmarkIsURLTargeted_Simple-18                            582244        4030 ns/op   12080 B/op   123 allocs/op
BenchmarkIsURLTargeted_Regex-18                             919930        2571 ns/op    8183 B/op    93 allocs/op
```

Allocation counts are unchanged. Single-run timings do not establish a speedup
or regression.

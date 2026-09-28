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

## 2026-09-25 - Saved group references v2 (PR #98)

- Base: `453db2f3b377abbc8f8d6e02c5bec0cd3f20948e`.
- Measured PR commit: `c15a0a6e63e205ed48a7f814c01861a26776f671`; subsequent
  commit `d39ef8e` adds tests only.
- Go: `go1.27.1 darwin/arm64`; CPU: Apple M5 Pro; GOMAXPROCS: 18.
- Scope: v2 list/condition references, attribute overrides, branch-local cycle
  tracking, legacy membership compatibility, and encrypted saved-group loading.
- Method: six samples per version, run sequentially with alternating base/PR
  order. Values below are medians, not formal statistical significance estimates.

Existing root benchmarks used compiled test binaries with
`-test.run='^$' -test.bench=. -test.benchmem -test.benchtime=300ms`.
The condition package used the same flags with `-test.benchtime=200ms`.

| Benchmark | Base ns/op | PR ns/op | Change | Allocs/op (unchanged) |
| --- | ---: | ---: | ---: | ---: |
| EvalFeature_Cold | 85.94 | 84.89 | -1.2% | 3 |
| EvalFeature_Warm | 92.10 | 94.11 | +2.2% | 3 |
| EvalFeature_ObjectValue_ExperimentCallback | 67.45 | 67.76 | +0.5% | 3 |
| EvalFeature_ObjectValue_FeatureUsageCallback | 184.30 | 184.15 | -0.1% | 6 |
| RunExperiment | 218.95 | 221.40 | +1.1% | 5 |
| EvalFeature_Parallel | 214.65 | 213.10 | -0.7% | 3 |
| IsURLTargeted_Simple | 4471.50 | 4501.00 | +0.7% | 123 |
| IsURLTargeted_Regex | 2829.50 | 2824.00 | -0.2% | 93 |

These benchmarks do not exercise saved-group lookup. Existing condition
benchmarks remained allocation-free, with median differences from -7.2% to
+4.9% (at most 1.20 ns).

### Focused saved-group measurements

Temporary review benchmarks (not checked into the repository) exercised
last-element hits and misses in lists of 1, 100, and 10,000 strings, targeting
conditions, and JSON payload updates. Evaluation used 300 ms samples; loading
was confirmed with six alternating 1 s samples per version.

| Operation | Base | PR | Change |
| --- | ---: | ---: | ---: |
| Targeting with multiple conditions | 137.10 ns | 140.95 ns | +2.8% |
| Legacy group, 10,000 strings, last-element hit | 51.29 us | 50.47 us | -1.6% |
| Load legacy group, 10,000 strings | 1.257 ms | 1.375 ms | +9.4% |

Legacy evaluation retained 256 B and 3 allocations per call. Loading the
10,000-member group increased from 2,096,615 to 2,219,627 B/op (+5.9%) and
40,106 to 40,110 allocations. The loader now stages raw JSON, decodes entries
separately, and copies the group map during normalization. This cost occurs
during payload updates, not each evaluation.

New v2 string-list evaluation measured 161–200 ns across these list sizes,
with 512 B and 5 allocations per call. A condition group referencing the same
list twice measured 375 ns, 1,024 B, and 9 allocations. These are costs of the
new capability, not comparisons against a previous v2 implementation.

A follow-up per-entry decoder experiment reduced legacy loading memory by
5.5% but did not improve speed, so it was reverted. The results above describe
the retained implementation; no further implementation changes were recommended.

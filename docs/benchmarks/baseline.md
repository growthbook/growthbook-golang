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

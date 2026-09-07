module github.com/growthbook/growthbook-golang/stickybucket/redis

go 1.22

toolchain go1.22.7

// go-redis is pinned below v9.15: from v9.15 onwards its go directive is 1.24,
// which would stop anyone on the Go version the SDK itself supports from using
// this adapter at all. Bump it once the SDK's own floor moves.
require (
	github.com/growthbook/growthbook-golang v0.4.0
	github.com/redis/go-redis/v9 v9.14.1
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tmaxmax/go-sse v0.10.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// The adapter is developed against the SDK in this repository. Consumers ignore
// replace directives in a dependency, so they resolve the require above.
replace github.com/growthbook/growthbook-golang => ../..

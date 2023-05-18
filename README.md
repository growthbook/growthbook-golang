![](growthbook-hero-go-sdks.png)

# GrowthBook Go SDK

- [Requirements](#requirements)
- [Installation](#installation)
- [Documentation](#documentation)


## Requirements

- Go version 1.17 or higher


## Installation

```
go get github.com/growthbook/growthbook-golang
```

## Documentation

- [Usage Guide](https://docs.growthbook.io/lib/go)
- [godoc](https://growthbook.github.io/growthbook-golang)


## JSON support

Most types used in the SDK have functions to build values from
representations as JSON objects, e.g. `ParseExperiment`,
`BuildFeatureRule`, etc. These functions are useful both for testing
and for user creation of GrowthBook objects from JSON configuration
data shared with GrowthBook SDK implementations in other languages,
all of which use JSON as a common configuration format.

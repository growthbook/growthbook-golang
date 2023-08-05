# Changelog

All notable changes to this project will be documented in this file.

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

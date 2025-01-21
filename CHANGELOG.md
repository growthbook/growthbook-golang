# Changelog

All notable changes to this project will be documented in this file.

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

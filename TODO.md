- Implement subscriptions.

- Implement getters and setters for public types in a tidier way.

- Standard Go documentation everywhere.

- In addition to the JSON tests, write custom test cases for: event
  subscriptions, tracking callbacks, getters/setters, logging, etc.

- Review all Go types for:
   * Optional/required values
   * Appropriate use of interface{}
   
- Come up with clear policy for error handling, including a
  distinction between development and production environments: strict
  in development, lax in production. (Deal with this by communicating
  all errors via out-of-band logging interface, so that the main API
  functions can be error-free in all cases?)

- Look at the public APIs and think about how to give the best Go-like
  DX there.

- Tidy up the whole story with converting data values from JSON.

- Improve the conditions code: more type safety, error logging, what
  else?

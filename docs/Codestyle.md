# Style guide
We strive to write idiomatic code during the development of our project.

We have several artifacts that outline recommendations for code writing. For example:

https://google.github.io/styleguide/go/
https://go.dev/doc/effective_go
https://github.com/golang/go/wiki/CodeReviewComments
Below are listed some rules where we deviate from these recommendations.

## Rules

### No underscores in file names
Use underscores only for test files and for separating platform specific (build tag constrained) implementations:
- `foo_test.go`
- `foo_windows.go`
- `foo_heic.go` 

### Variable names
https://github.com/golang/go/wiki/CodeReviewComments#variable-names

We give variables meaningful names. Avoid using single-letter names, except for loop counters.

By looking at a variable, its purpose should be evident.

We prefer `lineCount` over `c` or `lc`.

Our experience demonstrates that such code is easier to comprehend and contains fewer errors.

# Style guide
We strive to write idiomatic code during the development of our project.

We have several artifacts that outline recommendations for code writing. For example:

https://google.github.io/styleguide/go/
https://go.dev/doc/effective_go
https://github.com/golang/go/wiki/CodeReviewComments
Below are listed some rules where we deviate from these recommendations.

## Rules
### Package names
https://go.dev/doc/effective_go#package-names

We use `_` (snake_case) as word separators in package names.
Our experience shows that this approach provides better clarity.

Example: `package_name`, not `packagename`.

### Variable names
https://github.com/golang/go/wiki/CodeReviewComments#variable-names

We give variables meaningful names. Avoid using single-letter names, except for loop counters.

By looking at a variable, its purpose should be evident.

We prefer `lineCount` over `c` or `lc`.

Our experience demonstrates that such code is easier to comprehend and contains fewer errors.
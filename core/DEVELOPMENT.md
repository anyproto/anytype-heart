# Development

## Services/components
### Bootstrapping
If you need your component to be visible to other components, you need to register it in Service Locator's registry. 
To do it go to `Bootstrap` function in `core/anytype/bootstrap.go` and `Register` it

### Dependency injection
We use our own implementation of Service Locator pattern: `github.com/anyproto/any-sync/app`. 

## Writing tests

### Structure of tests
Prefer structuring your tests in Act-Arrange-Assert style. Use comments for visual separation of those test parts, like:
```go
// Given
...
// When
...
// Then
```

### Fixtures for services under test
Define `fixture` structure to easily bootstrap service and its dependencies. You can find examples in our code.

### Mocking
Prefer using Mockery for new mocks. It's configured in `.mockery.yaml`

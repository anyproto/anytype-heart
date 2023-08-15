# Testing
## Run tests
Install dependencies for running tests and generate mocks:
```
make test-deps
```

GO test:
```
make test
```
You'll need to install latest (at least clang 15)
```
brew install llvm 
echo 'export PATH="/<homebrew location>/llvm/bin:$PATH"' >> ~/.zshrc 
```

### Integration tests
First you need to start a docker container via docker-compose:
```
export ANYTYPE_TEST_GRPC_PORT=31088
docker-compose up -d
```

Then you can run the basic integration tests:
```
make test-integration
```

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
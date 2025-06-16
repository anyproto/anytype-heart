# tests
test:
	@echo 'Running tests...'
	@ANYTYPE_LOG_NOGELF=1 go test -cover github.com/anyproto/anytype-heart/...

test-no-cache:
	@echo 'Running tests...'
	@CGO_LDFLAGS=-Wl,-no_warn_duplicate_libraries ANYTYPE_LOG_NOGELF=1 go test -count=1 github.com/anyproto/anytype-heart/...

test-integration:
	@echo 'Running integration tests...'
	@go test -run=TestBasic -tags=integration -v -count 1 ./tests

test-race:
	@echo 'Running tests with race-detector...'
	@CGO_LDFLAGS=-Wl,-no_warn_duplicate_libraries ANYTYPE_LOG_NOGELF=1 go test -count=1 -race github.com/anyproto/anytype-heart/...

test-deps:
	@echo 'Generating test mocks...'
	@go build -o deps go.uber.org/mock/mockgen
	@go build -o deps github.com/vektra/mockery/v2
	@go generate ./...
	@$(DEPS_PATH)/mockery --disable-version-string
	@go run ./cmd/testcase generate-json-helpers

clear-test-deps:
	@echo 'Removing test mocks...'
	@find . -name "*_mock.go" | xargs -r rm -v

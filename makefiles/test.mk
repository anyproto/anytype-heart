# tests
UNAME_S := $(shell uname -s)

ifeq ($(UNAME_S),Darwin)
    CGO_LDFLAGS_DARWIN = CGO_LDFLAGS=-Wl,-no_warn_duplicate_libraries
endif

test:
	@echo 'Running tests...'

	@$(CGO_LDFLAGS_DARWIN) GOEXPERIMENT=synctest ANYTYPE_LOG_NOGELF=1 go test github.com/anyproto/anytype-heart/...

test-no-cache:
	@echo 'Running tests...'
	@$(CGO_LDFLAGS_DARWIN) GOEXPERIMENT=synctest ANYTYPE_LOG_NOGELF=1 go test -count=1 github.com/anyproto/anytype-heart/...

test-integration:
	@echo 'Running integration tests...'
	@go test -run=TestBasic -tags=integration -v -count 1 ./tests

test-race:
	@echo 'Running tests with race-detector...'
	@$(CGO_LDFLAGS_DARWIN) GOEXPERIMENT=synctest ANYTYPE_LOG_NOGELF=1 go test -count=1 -race github.com/anyproto/anytype-heart/...

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

test-failed:
	@echo 'Running tests and showing only failures...'
	@set -o pipefail; \
	CGO_LDFLAGS=-Wl,-no_warn_duplicate_libraries ANYTYPE_LOG_NOGELF=1 GOEXPERIMENT=synctest go test -v github.com/anyproto/anytype-heart/... 2>&1 | \
	awk '/^=== RUN|^--- FAIL:|^FAIL|Error:|error:|panic:|\t.*\.go:[0-9]+:/ { \
		if ($$0 ~ /^=== RUN/) { current_test = $$0 } \
		else { if (current_test != "") { print current_test; current_test = "" } print $$0 } \
	} \
	END { if (NR == 0) print "All tests passed!" }'

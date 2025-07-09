install-linter:
	@go install github.com/daixiang0/gci@latest
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

run-linter:
ifdef GOLANGCI_LINT_BRANCH
	@golangci-lint run -v ./... --new-from-rev=$(GOLANGCI_LINT_BRANCH) --timeout 15m --verbose
else
	@golangci-lint run -v ./... --new-from-rev=origin/main --timeout 15m --verbose
endif

run-linter-fix:
ifdef GOLANGCI_LINT_BRANCH
	@golangci-lint run -v ./... --new-from-rev=$(GOLANGCI_LINT_BRANCH) --timeout 15m --fix
else
	@golangci-lint run -v ./... --new-from-rev=origin/main --timeout 15m --fix
endif

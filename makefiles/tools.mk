fmt:
	@echo 'Formatting with prettier...'
	@npx prettier --write "./**" 2> /dev/null || true
	@echo 'Formatting with goimports...'
	@goimports -w -l `find . -type f -name '*.go' -not -path './vendor/*'`

lint:
	@echo 'Linting with prettier...'
	@npx prettier --check "./**" 2> /dev/null || true
	@echo 'Linting with golint...'
	@golint `go list ./... | grep -v /vendor/`

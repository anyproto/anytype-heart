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

swagger:
	@echo 'Generating swagger docs...'
	@swag init --v3.1 -q -d core/api -g service.go -o core/api/docs
	@echo 'Formatting swagger docs...'
	@swag fmt -d core/api

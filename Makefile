# include makefiles
include $(wildcard makefiles/*.mk)

# git config
$(shell git config core.hooksPath .githooks)

.PHONY: $(MAKECMDGOALS)

run-server: build-server
	@echo 'Running server...'
	@./dist/server

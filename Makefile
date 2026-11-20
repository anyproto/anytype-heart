# include makefiles
include $(wildcard makefiles/*.mk)


.PHONY: $(MAKECMDGOALS)

run-server: build-server
	@echo 'Running server...'
	@./dist/server

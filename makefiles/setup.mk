setup: setup-go
	@echo 'Setting up npm...'
	@npm install

setup-network-config:
ifdef ANYENV
	@echo "ANYENV is now deprecated. Use ANY_SYNC_NETWORK instead."
	@exit 1;
endif
	@if [ -z "$$ANY_SYNC_NETWORK" ]; then \
	echo "Using the default production Any Sync Network"; \
elif [ ! -e "$$ANY_SYNC_NETWORK" ]; then \
	echo "Network configuration file not found at $$ANY_SYNC_NETWORK"; \
	exit 1; \
else \
	echo "Using Any Sync Network configuration at $$ANY_SYNC_NETWORK"; \
	cp $$ANY_SYNC_NETWORK $(CUSTOM_NETWORK_FILE); \
fi

setup-go: setup-network-config check-tantivy-version
	@echo 'Setting up go modules...'
	@go mod download
	@go install github.com/ahmetb/govvv@v0.2.0

setup-gomobile:
	go build -o deps golang.org/x/mobile/cmd/gomobile
	go build -o deps golang.org/x/mobile/cmd/gobind
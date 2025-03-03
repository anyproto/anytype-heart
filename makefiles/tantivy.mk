### Tantivy Section

REPO := anyproto/tantivy-go
OUTPUT_DIR := deps/libs
SHA_FILE = tantivity_sha256.txt

TANTIVY_LIBS := android-386.tar.gz \
	android-amd64.tar.gz \
	android-arm.tar.gz \
	android-arm64.tar.gz \
	darwin-amd64.tar.gz \
	darwin-arm64.tar.gz \
	ios-amd64.tar.gz \
	ios-arm64.tar.gz \
	ios-arm64-sim.tar.gz \
	linux-amd64-musl.tar.gz \
	linux-arm64-musl.tar.gz \
	windows-amd64.tar.gz

define download_tantivy_lib
	curl -L -o $(OUTPUT_DIR)/$(1) https://github.com/$(REPO)/releases/download/$(TANTIVY_VERSION)/$(1)
endef

define remove_arch
	rm -f $(OUTPUT_DIR)/$(1)
endef

remove-libs:
	@rm -rf deps/libs/*

write-tantivy-version:
	@echo "$(TANTIVY_VERSION)" > $(OUTPUT_DIR)/.verified

download-tantivy: remove-libs $(TANTIVY_LIBS)

$(TANTIVY_LIBS):
	@mkdir -p $(OUTPUT_DIR)/$(shell echo $@ | cut -d'.' -f1)
	$(call download_tantivy_lib,$@)
	@tar -C $(OUTPUT_DIR)/$(shell echo $@ | cut -d'.' -f1) -xvzf $(OUTPUT_DIR)/$@

download-tantivy-all-force: download-tantivy
	rm -f $(SHA_FILE)
	@for file in $(TANTIVY_LIBS); do \
		echo "SHA256 $(OUTPUT_DIR)/$$file" ; \
		shasum -a 256 $(OUTPUT_DIR)/$$file | awk '{print $$1 "  " "'$(OUTPUT_DIR)/$$file'" }' >> $(SHA_FILE); \
	done
	@rm -rf deps/libs/*.tar.gz
	@echo "SHA256 checksums generated."
	$(MAKE) write-tantivy-version

download-tantivy-all: download-tantivy
	@echo "Validating SHA256 checksums..."
	@shasum -a 256 -c $(SHA_FILE) --status || { echo "Hash mismatch detected. Call make download-tantivy-all-force"; exit 1; }
	@echo "All files are valid."
	@rm -rf deps/libs/*.tar.gz
	$(MAKE) write-tantivy-version

download-tantivy-local: remove-libs
	@mkdir -p $(OUTPUT_DIR)
	@cp -r $(TANTIVY_GO_PATH)/libs/* $(OUTPUT_DIR)

check-tantivy-version:
	$(eval OLD_VERSION := $(shell [ -f $(OUTPUT_DIR)/.verified ] && cat $(OUTPUT_DIR)/.verified || echo ""))
	@if [ "$(TANTIVY_VERSION)" != "$(OLD_VERSION)" ]; then \
		$(MAKE) download-tantivy-all; \
	fi

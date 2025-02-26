PACK_SERVER_OS_ARCHS = windows-amd64 darwin-amd64 darwin-arm64 linux-amd64 linux-arm64
prepare-pack-server:
	mkdir -p .release/

pack-server-%:
	@OSARCH=$*; \
	if [ "$$OSARCH" = "windows-amd64" ]; then \
		BINARY_NAME=grpc-server.exe; \
		ARCHIVE_CMD="zip -r"; \
		ARCHIVE_FILE="js_$(VERSION)_$$OSARCH.zip"; \
	else \
		BINARY_NAME=grpc-server; \
		ARCHIVE_CMD="tar -czf"; \
		ARCHIVE_FILE="js_$(VERSION)_$$OSARCH.tar.gz"; \
	fi; \
	cp ./$$OSARCH* ./$$BINARY_NAME; \
	$$ARCHIVE_CMD $$ARCHIVE_FILE $$BINARY_NAME protobuf json; \
	mv $$ARCHIVE_FILE .release/

pack-server: prepare-pack-server $(addprefix pack-server-,$(PACK_SERVER_OS_ARCHS))

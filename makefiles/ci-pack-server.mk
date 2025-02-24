PACK_SERVER_OS_ARCHS = windows-amd64 darwin-amd64 darwin-arm64 linux-amd64
prepare-pack-server:
	mkdir -p .release/

pack-server-%:
	@OSARCH=$*; \
	if [ "$$OSARCH" = "windows-amd64" ]; then \
		BINARY_NAME=grpc-server.exe; \
	else \
		BINARY_NAME=grpc-server; \
	fi; \
	cp ./$$OSARCH* ./$$BINARY_NAME; \
	tar -czf js_$(VERSION)_$$OSARCH.tar.gz $$BINARY_NAME protobuf json; \
	mv js_$(VERSION)_$$OSARCH.tar.gz .release/

pack-server: prepare-pack-server $(addprefix pack-server-,$(PACK_SERVER_OS_ARCHS))

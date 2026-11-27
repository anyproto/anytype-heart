PACK_SERVER_OS_ARCHS = windows-amd64 darwin-amd64 darwin-arm64 linux-amd64 linux-arm64
prepare-pack-server:
	mkdir -p .release/

# js_<version>_<os-arch>: the server and both anytype-mcp builds, with protobuf/ and json/. Each
# binary is copied by its exact name into a fresh folder (a glob would match all three), and the
# first failed copy stops the recipe.
pack-server-%:
	@set -e; \
	OSARCH=$*; \
	EXT=; \
	if [ "$$OSARCH" = "windows-amd64" ]; then EXT=.exe; fi; \
	DIR=.pack/$$OSARCH; \
	rm -rf $$DIR; \
	mkdir -p $$DIR; \
	cp ./$$OSARCH $$DIR/grpc-server$$EXT; \
	cp ./$$OSARCH-mcp-dev $$DIR/anytype-mcp-dev$$EXT; \
	cp ./$$OSARCH-mcp-release $$DIR/anytype-mcp-release$$EXT; \
	cp -R protobuf json $$DIR/; \
	MEMBERS="grpc-server$$EXT anytype-mcp-dev$$EXT anytype-mcp-release$$EXT protobuf json"; \
	if [ "$$OSARCH" = "windows-amd64" ]; then \
		(cd $$DIR && zip -qr "$(CURDIR)/.release/js_$(VERSION)_$$OSARCH.zip" $$MEMBERS); \
	else \
		(cd $$DIR && COPYFILE_DISABLE=1 tar -czf "$(CURDIR)/.release/js_$(VERSION)_$$OSARCH.tar.gz" $$MEMBERS); \
	fi; \
	rm -rf $$DIR

pack-server: prepare-pack-server $(addprefix pack-server-,$(PACK_SERVER_OS_ARCHS))

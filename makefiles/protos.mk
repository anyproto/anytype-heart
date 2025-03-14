ROOT:=${PWD}

comma:=,

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

ifeq ($(UNAME_S), Darwin)
    PROTOC_OS = osx
else
    PROTOC_OS = linux
endif

ifeq ($(UNAME_M), x86_64)
    PROTOC_ARCH = x86_64
else
    PROTOC_ARCH = aarch_64
endif

PROTOC_ZIP = protoc-$(PROTOC_VERSION)-$(PROTOC_OS)-$(PROTOC_ARCH).zip

export DEPS=${ROOT}/deps
export GOGO_NO_UNDERSCORE=1
export GOGO_GOMOBILE_WITH_CONTEXT=1
export GOGO_EXPORT_ONEOF_INTERFACE=1
export PACKAGE_PATH=github.com/anyproto/anytype-heart/pb

PROTOC_VERSION = 29.3

PROTOC_URL = https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP)

PROTOC = $(DEPS)/protoc
PROTOC_GEN_GO = $(DEPS)/protoc-gen-go
PROTOC_GEN_DRPC = $(DEPS)/protoc-gen-go-drpc
PROTOC_GEN_VTPROTO = $(DEPS)/protoc-gen-go-vtproto
PROTOC_INCLUDE := $(DEPS)/include

define generate_proto
	@echo "Generating Protobuf for directory: $(1)"
	$(PROTOC) \
		--proto_path=. \
		--proto_path=$(PROTOC_INCLUDE) \
		--go_out=:. --plugin protoc-gen-go="$(PROTOC_GEN_GO)" \
		--go-vtproto_out=. --plugin protoc-gen-go-vtproto="$(PROTOC_GEN_VTPROTO)" \
		--go-vtproto_opt=features=marshal+unmarshal+size+equal \
		--go_opt=paths=source_relative \
		--go-vtproto_opt=paths=source_relative \
		--go_opt=$(1) \
		$(wildcard $(2)/*.proto)
endef

define generate_proto_drpc2
	@echo "Generating Protobuf for directory: $(1)"
	$(PROTOC) \
		--proto_path=. \
		--proto_path=$(PROTOC_INCLUDE) \
		--plugin protoc-gen-go-drpc="$(PROTOC_GEN_DRPC)" \
		--go_out=:. --plugin protoc-gen-go="$(PROTOC_GEN_GO)" \
		--go-vtproto_out=. --plugin protoc-gen-go-vtproto="$(PROTOC_GEN_VTPROTO)" \
		--go-vtproto_opt=features=marshal+unmarshal+size+equal \
		--go_opt=paths=source_relative \
		--go-vtproto_opt=paths=source_relative \
		--go-drpc_out=protolib=github.com/planetscale/vtprotobuf/codec/drpc:. \
		--go_opt=$(1) \
		$(wildcard $(2)/*.proto)
endef

define generate_proto_drpc
	@echo "Generating Protobuf for directory: $(1)"
	$(PROTOC) \
		--go_out=. --plugin protoc-gen-go="$(PROTOC_GEN_GO)" \
		--plugin protoc-gen-go-drpc="$(PROTOC_GEN_DRPC)" \
		--go_opt=$(1) \
		--go-vtproto_out=:. --plugin protoc-gen-go-vtproto="$(PROTOC_GEN_VTPROTO)" \
		--go-vtproto_opt=features=marshal+unmarshal+size \
		--go-drpc_out=protolib=github.com/planetscale/vtprotobuf/codec/drpc:. $(wildcard $(2)/*.proto)
endef

define generate_proto_grpc
	@echo "Generating Protobuf for directory: $(1)"
	$(PROTOC) \
		--proto_path=. \
		--proto_path=$(PROTOC_INCLUDE) \
		--go_out=:. --plugin protoc-gen-go="$(PROTOC_GEN_GO)" \
		--go-vtproto_out=. --plugin protoc-gen-go-vtproto="$(PROTOC_GEN_VTPROTO)" \
		--go-vtproto_opt=features=grpc+marshal+unmarshal+size+equal \
		--go_opt=paths=source_relative \
		--go-vtproto_opt=paths=source_relative \
		--go_opt=$(1) \
		$(wildcard $(2)/*.proto)
endef

define generate_proto_mobile
	@echo "Generating Protobuf for directory: $(1)"
	$(PROTOC) \
		--proto_path=. \
		--proto_path=$(PROTOC_INCLUDE) \
		--go_out=. --plugin protoc-gen-go="$(PROTOC_GEN_GO)" \
		--go-vtproto_out=:. --plugin protoc-gen-go-vtproto="$(PROTOC_GEN_VTPROTO)" \
		--go-vtproto_opt=features=gomobile \
		--go_opt=$(1) \
		$(wildcard $(2)/*.proto)
endef

define generate_proto_docs
	@echo "Generating Protobuf docs for directory: $(1)"
	$(PROTOC) \
		--doc_out=./docs --doc_opt=markdown,proto.md \
		$(wildcard $(1)/*.proto) $(wildcard $(2)/*.proto) $(wildcard $(3)/*.proto)
endef

protos-server:
	@echo 'Generating protobuf packages for lib-server (Go)...'
	@$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=google.golang.org/protobuf/jsonpbtypes)
	@$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=google.golang.org/protobuf/jsonpbtypes)
	@$(eval P_DESCRIPTOR := Mgoogle/protobuf/descriptor.proto=google.golang.org/protobuf/protoc-gen-gogo/descriptor)
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_DESCRIPTOR))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 GOGO_GRPC_SERVER_METHOD_NO_ERROR=1 PACKAGE_PATH=github.com/anyproto/anytype-heart/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=grpc:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./pb/service/

protos-go:
	@echo 'Generating protobuf packages for lib (Go)...'
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=google.golang.org/protobuf/jsonpbtypes)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=google.golang.org/protobuf/jsonpbtypes)
	@$(eval P_DESCRIPTOR := Mgoogle/protobuf/descriptor.proto=google.golang.org/protobuf/protoc-gen-gogo/descriptor)
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpkg/lib/pb/model/protos/localstore.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)

	$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_DESCRIPTOR))
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pkg/lib/pb/ pkg/lib/pb/model/protos/*.proto; mv pkg/lib/pb/pkg/lib/pb/model/*.go pkg/lib/pb/model/; rm -rf pkg/lib/pb/pkg
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP):./pkg/lib/pb/ pkg/lib/pb/storage/protos/*.proto; mv pkg/lib/pb/pkg/lib/pb/storage/*.go pkg/lib/pb/storage/; rm -rf pkg/lib/pb/pkg
	GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc --gogofaster_out=$(PKGMAP),plugins=grpc:./ pkg/lib/cafe/pb/*.proto
	@echo 'Generating protobuf packages for mw (Go)...'
	@$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=google.golang.org/protobuf/jsonpbtypes)
	@$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=google.golang.org/protobuf/jsonpbtypes)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_DESCRIPTOR))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 protoc -I . --gogofaster_out=$(PKGMAP):. ./pb/protos/*.proto; mv ./pb/protos/*.pb.go ./pb/
	@$(eval P_PROTOS4 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS5 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_PROTOS4),$$(P_PROTOS5),$$(P_DESCRIPTOR))
	@GOGO_NO_UNDERSCORE=1 GOGO_GOMOBILE_WITH_CONTEXT=1 GOGO_EXPORT_ONEOF_INTERFACE=1 PACKAGE_PATH=github.com/anyproto/anytype-heart/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./clientlibrary/service/
	@protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos-gomobile:
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	@GOGO_NO_UNDERSCORE=1 GOGO_EXPORT_ONEOF_INTERFACE=1 GOGO_GOMOBILE_WITH_CONTEXT=1 PACKAGE_PATH=github.com/anyproto/anytype-heart/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./clientlibrary/service/

protos-docs:
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	@protoc -I ./ --doc_out=./docs --doc_opt=markdown,proto.md pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos: protos-go protos-server protos-docs

protos-swift:
	@echo 'Clear protobuf files'
	@rm -rf ./dist/ios/protobuf/*
	@echo 'Generating swift protobuf files'
	@protoc -I ./  --swift_opt=FileNaming=DropPath --swift_opt=Visibility=Public --swift_out=./dist/ios/protobuf pb/protos/*.proto pkg/lib/pb/model/protos/*.proto
	@echo 'Generated swift protobuf files at ./dist/ios/pb'

protos-swift-local: protos-swift
	@echo 'Clear proto files'
	@rm -rf ./dist/ios/protobuf/protos
	@rm -rf ./dist/ios/protobuf/protos
	@echo 'Copying proto files'
	@mkdir ./dist/ios/protobuf/protos
	@cp ./pb/protos/*.proto ./dist/ios/protobuf/protos
	@cp ./pb/protos/service/*.proto ./dist/ios/protobuf/protos
	@cp ./pkg/lib/pb/model/protos/*.proto ./dist/ios/protobuf/protos
	@open ./dist

protos-js:
	@echo 'Generating protobuf packages (JS)...'
	@protoc -I ./  --js_out=import_style=commonjs,binary:./dist/js/pb pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto
		@protoc -I ./  --grpc-web_out=import_style=commonjs+dts,mode=grpcwebtext:./dist/js/pb pb/protos/service/*.proto pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

protos-java:
	@echo 'Generating protobuf packages (Java)...'
	@protoc -I ./ --java_out=./dist/android/pb pb/protos/*.proto pkg/lib/pb/model/protos/*.proto

VT_PROTOBUF_REPO := /Users/mikhailyudin/GolandProjects/vtprotobuf
GO_PROTOBUF_REPO := /Users/mikhailyudin/GolandProjects/protobuf-go
PROTOC_GEN_VTPROTO := $(DEPS)/protoc-gen-go-vtproto

VT_PROTOBUF_COMMIT := 57a97b786bfdef686fce425af0b32376dedac8ce
GO_PROTOBUF_COMMIT := d58efe595bddd808375cd0c4f66dafe33a11d8b0

fork1:
	@echo "Cloning vtprotobuf fork..."
	@rm -rf VT_PROTOBUF_REPO
	git clone https://github.com/anyproto/vtprotobuf.git $(VT_PROTOBUF_REPO) || true
	cd $(VT_PROTOBUF_REPO) && git fetch && git checkout $(VT_PROTOBUF_COMMIT)
	@echo "Building protoc-gen-go-vtproto..."
	@echo "Cloning protoc-gen-go fork..."
	@rm -rf GO_PROTOBUF_REPO
	git clone https://github.com/anyproto/protobuf-go.git $(GO_PROTOBUF_REPO) || true
	cd $(GO_PROTOBUF_REPO) && git fetch && git checkout $(GO_PROTOBUF_COMMIT)
	@echo "Building protoc-gen-go..."
	GOBIN=$(DEPS) go install storj.io/drpc/cmd/protoc-gen-go-drpc@latest
	go build -o deps github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc
	cd $(VT_PROTOBUF_REPO)/cmd/protoc-gen-go-vtproto && go build -o $(PROTOC_GEN_VTPROTO)
	cd $(GO_PROTOBUF_REPO)/cmd/protoc-gen-go && go build -o $(PROTOC_GEN_GO)

deps2: fork1
	go mod download
	@echo "Downloading protoc $(PROTOC_VERSION)..."
	curl -OL $(PROTOC_URL)
	mkdir -p $(DEPS)
	unzip -o $(PROTOC_ZIP) -d $(ROOT)
	mv bin/protoc $(DEPS)
	rm $(PROTOC_ZIP)
	mv include deps
	rm -rf readme.txt
	rm -rf bin
	@echo "protoc installed in $(DEPS)/bin"

	GOBIN=$(DEPS) go install storj.io/drpc/cmd/protoc-gen-go-drpc@latest
	GOBIN=$(DEPS) go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	#GOBIN=$(DEPS) go install github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto@latest

@GOGO_NO_UNDERSCORE=1 GOGO_GOMOBILE_WITH_CONTEXT=1 GOGO_EXPORT_ONEOF_INTERFACE=1 PACKAGE_PATH=github.com/anyproto/anytype-heart/pb protoc -I=. --gogofaster_out=$(PKGMAP),plugins=gomobile:. ./pb/protos/service/service.proto; mv ./pb/protos/service/*.pb.go ./clientlibrary/service/

lolka2: fork1
	@echo 'Generating protobuf packages for lib (Go)...'
	$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=google.golang.org/protobuf/jsonpbtypes)
	$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=google.golang.org/protobuf/jsonpbtypes)
	@$(eval P_DESCRIPTOR := Mgoogle/protobuf/descriptor.proto=google.golang.org/protobuf/protoc-gen-gogo/descriptor)
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpkg/lib/pb/model/protos/localstore.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)

	$(eval PKGMAP := $$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	$(call generate_proto,$(PKGMAP):./pkg/lib/pb/,pkg/lib/pb/model/protos/)
	mv pkg/lib/pb/model/protos/*.go pkg/lib/pb/model/;
	$(call generate_proto,$(PKGMAP):./pkg/lib/pb/,pkg/lib/pb/storage/protos)
	mv pkg/lib/pb/storage/protos/*.go pkg/lib/pb/storage/;
	$(call generate_proto_drpc,,space/spacecore/clientspaceproto/protos)
	mv commonspace/clientspaceproto/*.go space/spacecore/clientspaceproto;
	@echo 'Generating protobuf packages for mw (Go)...'
	@$(eval P_TIMESTAMP := Mgoogle/protobuf/timestamp.proto=google.golang.org/protobuf/jsonpbtypes)
	@$(eval P_STRUCT := Mgoogle/protobuf/struct.proto=google.golang.org/protobuf/jsonpbtypes)
	@$(eval PKGMAP:=$$(P_PROTOS)$(comma)$$(P_PROTOS2)$(comma)$$(P_PROTOS3))
	$(call generate_proto,$(PKGMAP),pb/protos)
	mv ./pb/protos/*.pb.go ./pb/
	@$(eval P_PROTOS4 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS5 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_TIMESTAMP),$$(P_STRUCT),$$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3),$$(P_PROTOS4),$$(P_PROTOS5),$$(P_DESCRIPTOR))
	$(call generate_proto_mobile,$(PKGMAP),pb/protos/service)
	$(call generate_proto_docs,pb/protos/service,pb/protos,pkg/lib/pb/model/protos)
	mv ./pb/protos/service/*.pb.go ./clientlibrary/service/

protos-server1: fork1
	@echo 'Generating protobuf packages for lib-server (Go)...'
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	$(call generate_proto_grpc,$(PKGMAP),pb/protos/service)
	mv ./pb/protos/service/*.pb.go ./pb/service/
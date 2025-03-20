ROOT:=${PWD}

comma:=,

export DEPS=${ROOT}/deps

PROTOC=$(shell which protoc)
PROTOC_GEN_GO=$(shell which protoc-gen-go)
PROTOC_GEN_DRPC=$(shell which protoc-gen-go-drpc)
PROTOC_GEN_VTPROTO=$(shell which protoc-gen-go-vtproto)

define generate_proto
	@echo "Generating Protobuf for directory: $(1)"
	$(PROTOC) \
		--proto_path=. \
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
	@$(eval P_PROTOS := Mpkg/lib/pb/model/protos/models.proto=github.com/anyproto/anytype-heart/pkg/lib/pb/model)
	@$(eval P_PROTOS2 := Mpb/protos/commands.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval P_PROTOS3 := Mpb/protos/events.proto=github.com/anyproto/anytype-heart/pb)
	@$(eval PKGMAP := $$(P_PROTOS),$$(P_PROTOS2),$$(P_PROTOS3))
	$(call generate_proto_grpc,$(PKGMAP),pb/protos/service)
	mv ./pb/protos/service/*.pb.go ./pb/service/

protos-go:
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
	mv ./pb/protos/service/*.pb.go ./clientlibrary/service/

protos-docs:
	$(call generate_proto_docs,pb/protos/service,pb/protos,pkg/lib/pb/model/protos)

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

ci-java-protos:
	make protos-java
	rm -rf protobuf
	mv dist/android/pb protobuf
	mkdir -p protobuf/protos
	cp pb/protos/*.proto ./protobuf/protos
	cp pb/protos/service/*.proto ./protobuf/protos
	cp pkg/lib/pb/model/protos/*.proto ./protobuf/protos

	# Add system relations/types jsons
	mkdir -p json/
	cp pkg/lib/bundle/systemRelations.json ./json
	cp pkg/lib/bundle/systemTypes.json ./json
	cp pkg/lib/bundle/internalRelations.json ./json
	cp pkg/lib/bundle/internalTypes.json ./json

{ stdenv
, lib
, fetchFromGitHub
, protobuf
, pkg-config
}:

stdenv.mkDerivation rec {
  pname = "protoc-gen-js";
  version = "4.0.0";

  src = fetchFromGitHub {
    owner = "protocolbuffers";
    repo  = "protobuf-javascript";
    rev   = "v${version}";
    hash  = "sha256-E647zdLrQK6rfmopS2eerQPdPk/YM/4sr5K6GyA/2Zw=";
  };

  nativeBuildInputs = [ pkg-config ];
  buildInputs       = [ protobuf ];

  protobufSrc = protobuf.src;

  buildPhase = ''
    runHook preBuild

    # Adjust flags if you want stricter warnings
    $CXX -std=c++17 -O2 \
      -I. \
      -I"$protobufSrc/src" \
      $(pkg-config --cflags protobuf) \
      generator/*.cc \
      $(pkg-config --libs protobuf) -lprotoc \
      -o protoc-gen-js

    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    install -Dm755 protoc-gen-js "$out/bin/protoc-gen-js"
    runHook postInstall
  '';

  meta = with lib; {
    description = "Protobuf plugin for generating JavaScript code (built from v${version})";
    homepage    = "https://github.com/protocolbuffers/protobuf-javascript";
    licenses    = [ licenses.bsd3 licenses.asl20 ];
    platforms   = platforms.unix;
  };
}

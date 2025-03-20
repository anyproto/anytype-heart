{
  description = "";
  inputs = {
    nixpkgs.url = "https://flakehub.com/f/NixOS/nixpkgs/0.1.0.tar.gz";
    flake-utils.url = "github:numtide/flake-utils";
    drpc.url = "github:storj/drpc/v0.0.34";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      drpc,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfree = true;
          };
        };
        devShell = pkgs.mkShell {
          name = "anytype-heart";
          nativeBuildInputs = [
            pkgs.protoc-gen-grpc-web
            pkgs.protoc-gen-js
            pkgs.go_1_23
            pkgs.gox
            pkgs.protobuf3_21
            pkgs.pkg-config
            pkgs.pre-commit
            # todo: govvv, not packaged
          ];
        };

        protoc-gen-go-vtproto = pkgs.callPackage .nix/protoc-gen-go-vtproto.nix { };
        protoc-gen-go = pkgs.callPackage .nix/protoc-gen-go.nix { };
        protoc-gen-go-drpc = drpc.defaultPackage.${system};

        protosDevShell = pkgs.mkShell {
          name = "protoc";
          nativeBuildInputs = with pkgs; [
            go_1_23
            protobuf
            protoc-gen-go
            protoc-gen-go-drpc
            protoc-gen-go-vtproto
            protoc-gen-doc
            protoc-gen-js
            protoc-gen-grpc-web
          ];
          shellHook = ''
            export GOROOT="${pkgs.go_1_23}/share/go"
            export PATH="$GOROOT/bin:$PATH"
          '';
        };
      in
      {
        devShells = {
          default = devShell;
          protos = protosDevShell;
        };
      }
    );
}

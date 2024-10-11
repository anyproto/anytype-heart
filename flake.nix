{
  description = "";
  inputs.nixpkgs.url = "https://flakehub.com/f/NixOS/nixpkgs/0.1.0.tar.gz";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system: let
      pkgs = import nixpkgs {
        inherit system;
        config = { allowUnfree = true; };
      };
    in {
      devShell = pkgs.mkShell {
        name = "anytype-heart";
        nativeBuildInputs = [
          pkgs.protoc-gen-grpc-web
          pkgs.go_1_22
          pkgs.gox
          pkgs.protobuf3_20
          pkgs.pkg-config
          pkgs.pre-commit
          # todo: govvv, not packaged
        ];
      };
    });
}

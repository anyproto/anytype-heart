{ buildGoModule, fetchFromGitHub }:

buildGoModule rec {
  pname = "protoc-gen-go-vtproto";
  version = "latest";

  src = fetchFromGitHub {
    owner = "anyproto";
    repo = "vtprotobuf";
    rev = "d97553cb619452c9caf232a96e4fcd074c88a17";
    sha256 = "sha256-FNAIcA45Ph2r+PIzIxE+RKiHUmSAMS/JUzOg7YqfgG8=";
  };

  subPackages = [ "cmd/protoc-gen-go-vtproto" ];

  vendorHash = "sha256-ngrRvGYnZ/4cJarC0qLrfm84UvXFhRCo1ZcPWBovtDs=";
}

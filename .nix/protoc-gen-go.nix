{ buildGoModule, fetchFromGitHub }:

buildGoModule rec {
  pname = "protoc-gen-go";
  version = "latest";

  src = fetchFromGitHub {
    owner = "anyproto";
    repo = "protobuf-go";
    rev = "d58efe595bddd808375cd0c4f66dafe33a11d8b0";
    sha256 = "sha256-DB9kO+sI6jogY2gC165xr+9f8tEuxJMNhNp9mXwzXLs=";
  };

  subPackages = [ "cmd/protoc-gen-go" ];

  vendorHash = "sha256-nGI/Bd6eMEoY0sBwWEtyhFowHVvwLKjbT4yfzFz6Z3E=";
}

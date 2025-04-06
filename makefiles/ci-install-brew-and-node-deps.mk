install-brew-and-node-deps:
	curl https://raw.githubusercontent.com/Homebrew/homebrew-core/31b24d65a7210ea0a5689d5ad00dd8d1bf5211db/Formula/protobuf.rb --output protobuf.rb
	curl https://raw.githubusercontent.com/Homebrew/homebrew-core/d600b1f7119f6e6a4e97fb83233b313b0468b7e4/Formula/s/swift-protobuf.rb --output swift-protobuf.rb
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install ./protobuf.rb
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install --ignore-dependencies ./swift-protobuf.rb
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install mingw-w64
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install grpcurl
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew tap messense/macos-cross-toolchains && brew install x86_64-unknown-linux-musl && brew install aarch64-unknown-linux-musl
	npm i -g node-gyp

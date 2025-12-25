install-brew-and-node-deps:
	curl https://raw.githubusercontent.com/Homebrew/homebrew-core/f92f5e23b0a952ac3fae1962138145ee65a2d823/Formula/protobuf.rb --output protobuf.rb
	curl https://raw.githubusercontent.com/Homebrew/homebrew-core/f92f5e23b0a952ac3fae1962138145ee65a2d823/Formula/p/protoc-gen-js.rb --output protoc-gen-js.rb
	curl https://raw.githubusercontent.com/Homebrew/homebrew-core/f92f5e23b0a952ac3fae1962138145ee65a2d823/Formula/s/swift-protobuf.rb --output swift-protobuf.rb
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install ./protobuf.rb
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install --ignore-dependencies ./swift-protobuf.rb
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install --ignore-dependencies ./protoc-gen-js.r
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install mingw-w64
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew install grpcurl
	HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 brew tap messense/macos-cross-toolchains && brew install x86_64-unknown-linux-musl && brew install aarch64-unknown-linux-musl
	npm i -g node-gyp

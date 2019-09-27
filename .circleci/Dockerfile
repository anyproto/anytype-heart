FROM golang:1.12.9

# replace shell with bash so we can source files
RUN rm /bin/sh && ln -s /bin/bash /bin/sh

# install dependencies
RUN apt-get update \
  && apt-get install -y curl \
  && apt-get install -y mingw-w64 \
  && apt-get install -y zip \
  && curl -sL https://deb.nodesource.com/setup_10.x -o nodesource_setup.sh \
  && bash nodesource_setup.sh \
  && apt-get install nodejs \
  && curl -sSL "https://github.com/gotestyourself/gotestsum/releases/download/v0.3.5/gotestsum_0.3.5_linux_amd64.tar.gz" | sudo tar -xz -C /usr/local/bin gotestsum \
  && apt-get -y autoclean

# add global node modules to path
ENV PATH="/usr/lib/node_modules/bin:${PATH}"


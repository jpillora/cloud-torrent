FROM alpine:3.5
MAINTAINER dev@jpillora.com
# prepare go env
ENV GOPATH /go
ENV NAME cloud-torrent
ENV PACKAGE github.com/jpillora/$NAME
ENV PACKAGE_DIR $GOPATH/src/$PACKAGE
ENV GOLANG_VERSION 1.8.1
ENV GOLANG_SRC_URL https://golang.org/dl/go$GOLANG_VERSION.src.tar.gz
ENV GOLANG_SRC_SHA256 33daf4c03f86120fdfdc66bddf6bfff4661c7ca11c5da473e537f4d69b470e57
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH
# in one step (to prevent creating superfluous layers):
# 1. fetch and install temporary build programs,
# 2. build cloud-torrent alpine binary
# 3. remove build programs
RUN set -ex \
    && apk update \
	&& apk add ca-certificates \
	&& apk add --no-cache --virtual .build-deps \
		bash \
		gcc \
		musl-dev \
		openssl \
		git \
		go \
		curl \
	&& curl -s https://raw.githubusercontent.com/docker-library/golang/132cd70768e3bc269902e4c7b579203f66dc9f64/1.8/alpine/no-pic.patch -o /no-pic.patch \
	&& cat /no-pic.patch \
	&& export GOROOT_BOOTSTRAP="$(go env GOROOT)" \
	&& wget -q "$GOLANG_SRC_URL" -O golang.tar.gz \
	&& echo "$GOLANG_SRC_SHA256  golang.tar.gz" | sha256sum -c - \
	&& tar -C /usr/local -xzf golang.tar.gz \
	&& rm golang.tar.gz \
	&& cd /usr/local/go/src \
	&& patch -p2 -i /no-pic.patch \
	&& ./make.bash \
	&& mkdir -p $PACKAGE_DIR \
	&& git clone https://$PACKAGE.git $PACKAGE_DIR \
	&& cd $PACKAGE_DIR \
	&& go build -ldflags "-X main.VERSION=$(git describe --abbrev=0 --tags)" -o /usr/local/bin/$NAME \
	&& apk del .build-deps \
	&& rm -rf /no-pic.patch $GOPATH /usr/local/go
#run!
ENTRYPOINT ["cloud-torrent"]

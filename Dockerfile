FROM alpine:edge as builder
LABEL maintainer="dev@jpillora.com"
# prepare go env
ENV GOPATH /go
ENV NAME cloud-torrent
ENV PACKAGE github.com/jpillora/$NAME
ENV PACKAGE_DIR $GOPATH/src/$PACKAGE
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH
ENV CGO_ENABLED 0
# in one step (to prevent creating superfluous layers):
# 1. fetch and install temporary build programs,
# 2. build cloud-torrent alpine binary
# 3. remove build programs
RUN set -ex \
        && apk update \
        && apk add ca-certificates \
        && apk add --no-cache --virtual .build-deps \
        && apk add patch \
        bash \
        gcc \
        musl-dev \
        openssl \
        git \
        go \
        && mkdir -p $PACKAGE_DIR \
        && git clone https://$PACKAGE.git $PACKAGE_DIR \
        && cd $PACKAGE_DIR \
        && go build -ldflags "-X main.VERSION=$(git describe --abbrev=0 --tags)" -o /usr/local/bin/$NAME \
        && apk del .build-deps \
        && rm -rf $GOPATH /usr/local/go
#run!

FROM alpine:edge

COPY --from=builder /usr/local/bin/cloud-torrent /usr/local/bin

ENTRYPOINT ["cloud-torrent"]
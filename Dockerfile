FROM golang:1.12.6-alpine as builder
# prepare go env
ENV GOPATH /go
ENV NAME cloud-torrent
ENV PACKAGE github.com/jpillora/$NAME
ENV PACKAGE_DIR $GOPATH/src/$PACKAGE
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH
ENV CGO_ENABLED 0

RUN apk add --no-cache ca-certificates bash gcc musl-dev openssl git go curl
RUN	git clone https://$PACKAGE.git $PACKAGE_DIR
RUN	cd $PACKAGE_DIR && go build -ldflags "-X main.VERSION=$(git describe --abbrev=0 --tags)" -o /usr/local/bin/$NAME



FROM alpine:3.9
LABEL maintainer="jpillora <dev@jpillora.com>"

RUN apk add --no-cache ca-certificates openssl curl

COPY --from=builder /usr/local/bin/$NAME /

WORKDIR downloads

ENTRYPOINT ["/cloud-torrent"]

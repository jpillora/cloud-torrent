#!/bin/bash
BIN=cloud-torrent
GITVER=$(git describe --tags)

rm -fv ${BIN}_*

OS=$1
ARCH=$2
SUFFIX=""

if [[ -z $OS ]]; then
  OS=$(go env GOOS)
fi

if [[ -z $ARCH ]]; then
  ARCH=$(go env GOARCH)
fi

if [[ $OS == "windows" ]]; then
  SUFFIX=".exe"
fi

#go mod vendor
# CGO_ENABLED=0 GOARCH=$ARCH GOOS=$OS go build -o ${BIN}_${OS}_${ARCH}${SUFFIX} -ldflags "-s -w -X main.VERSION=$GITVER"
CGO_ENABLED=0 GOARCH=$ARCH GOOS=$OS go build -o ${BIN}_${OS}_${ARCH}${SUFFIX} -ldflags "-s -w -X main.VERSION=$GITVER"

#!/bin/bash
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__file="${__dir}/$(basename "${BASH_SOURCE[0]}")"
__base="$(basename ${__file} .sh)"
__root="$(cd "$(dirname "${__dir}")" && pwd)" # <-- change this as it depends on your app

BIN=cloud-torrent
GITVER=$(git describe --tags)

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

pushd $__dir/..
BINFILE=${BIN}_${OS}_${ARCH}${SUFFIX} 
rm -fv ${BIN}_*
CGO_ENABLED=0 GOARCH=$ARCH GOOS=$OS go build -o ${BINFILE} -ldflags "-s -w -X main.VERSION=$GITVER"
gzip -v -9 -k ${BINFILE}

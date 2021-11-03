#!/bin/bash
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__file="${__dir}/$(basename "${BASH_SOURCE[0]}")"
__base="$(basename ${__file} .sh)"
__root="$(cd "$(dirname "${__dir}")" && pwd)" # <-- change this as it depends on your app

BINPREFIX=""
BIN=cloud-torrent

if [[ -d ${BINLOCATION} ]]; then
	BINPREFIX=${BINLOCATION}/
fi

GITVER=$(git describe --tags)

OS=""
ARCH=""
SUFFIX=""
OSSUFFIX=""
PKGCMD=
CGO=1
GO_LDFLAGS="-s -w -X main.VERSION=$GITVER"
GO_TAGS=""

for arg in "$@"; do
case $arg in
	amd64)
		ARCH=amd64
		;;
	arm64)
		ARCH=arm64
		;;
	386)
		ARCH=386
		;;
	windows)
		OS=windows
		OSSUFFIX=.exe
		;;
	darwin)
		OS=darwin
		ARCH=amd64
		;;
	xz)
		PKGCMD=xz
    	;;
	gzip)
		PKGCMD=gzip
		;;
	purego)
		CGO=0
		SUFFIX=_static
		;;
	static)
		CGO=1
		SUFFIX=_static
		GO_LDFLAGS="${GO_LDFLAGS} -extldflags=-static"
		GO_TAGS='netgo osusergo sqlite_omit_load_extension'
		;;
esac
done


if [[ -z $OS ]]; then
  OS=$(go env GOOS)
fi

if [[ -z $ARCH ]]; then
  ARCH=$(go env GOARCH)
fi

pushd $__dir/..
BINFILE=${BINPREFIX}${BIN}_${OS}_${ARCH}${SUFFIX}${OSSUFFIX}
CGO_ENABLED=$CGO GOARCH=$ARCH GOOS=$OS \
	go build -o ${BINFILE} \
		-trimpath \
		-ldflags "${GO_LDFLAGS}" \
		-tags "${GO_TAGS}"
if [[ ! -f ${BINFILE} ]]; then
  echo "Build failed. Check with error message above."
  exit 1
fi

if [[ ! -z $PKGCMD ]]; then
  ${PKGCMD} -v -9 ${BINFILE}
fi

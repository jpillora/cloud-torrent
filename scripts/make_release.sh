#!/bin/bash
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__file="${__dir}/$(basename "${BASH_SOURCE[0]}")"
__base="$(basename ${__file} .sh)"
__root="$(cd "$(dirname "${__dir}")" && pwd)" # <-- change this as it depends on your app

BIN=cloud-torrent
GITVER=$(git describe --tags)

OS=""
ARCH=""
SUFFIX=""
PKGCMD=
NOSTATIC=
CGO=1

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
		SUFFIX=.exe
		;;
	xz)
		PKGCMD=xz
    	;;
	nostat)
		NOSTATIC=1
    	;;
	gzip)
		PKGCMD=gzip
		;;
	purego)
		CGO=0
		SUFFIX=_static
		;;
esac
done


if [[ -z $OS ]]; then
  OS=$(go env GOOS)
fi

if [[ -z $ARCH ]]; then
  ARCH=$(go env GOARCH)
fi

if [[ -z $NOSTATIC ]]; then
	pushd $__dir/../static
	sh generate.sh
	popd
fi

pushd $__dir/..
BINFILE=${BIN}_${OS}_${ARCH}${SUFFIX} 
CGO_ENABLED=$CGO GOARCH=$ARCH GOOS=$OS go build -o ${BINFILE} -trimpath -ldflags "-s -w -X main.VERSION=$GITVER"
if [[ ! -f ${BINFILE} ]]; then
  echo "Build failed. Check with error message above."
  exit 1
fi

if [[ -z $NOSTATIC ]]; then
	git checkout HEAD -- static/*
fi

if [[ ! -z $PKGCMD ]]; then
  ${PKGCMD} -v -9 ${BINFILE}
fi

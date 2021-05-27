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
PKGCMD=
NOSTATIC=
CGO=1
STASHED=0

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
	darwin)
		OS=darwin
		ARCH=amd64
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
	if [[ $(git status . --short | wc -l) -gt 0 ]]; then
		git stash
		STASHED=1
		echo "Current repo changed, stashed"
	fi
	sh generate.sh
	popd
fi

pushd $__dir/..
BINFILE=${BINPREFIX}${BIN}_${OS}_${ARCH}${SUFFIX} 
CGO_ENABLED=$CGO GOARCH=$ARCH GOOS=$OS go build -o ${BINFILE} -trimpath -ldflags "-s -w -X main.VERSION=$GITVER"
if [[ ! -f ${BINFILE} ]]; then
  echo "Build failed. Check with error message above."
  exit 1
fi

if [[ -z $NOSTATIC ]]; then
	git checkout HEAD -- static/*
	if [[ $STASHED -eq 1 ]]; then
		git stash pop
	fi
fi

if [[ ! -z $PKGCMD ]]; then
  ${PKGCMD} -v -9 ${BINFILE}
fi

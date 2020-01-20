#!/bin/bash
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__file="${__dir}/$(basename "${BASH_SOURCE[0]}")"
__base="$(basename ${__file} .sh)"
__root="$(cd "$(dirname "${__dir}")" && pwd)" # <-- change this as it depends on your app

BIN=cloud-torrent
GITVER=$(git describe --tags)

OS=""
ARCH=""
EXESUFFIX=""
PKGCMD=
NOSTATIC=

for arg in "$@"; do
case $arg in
	386)
		GOARCH=386
		;;
	windows)
		OS=windows
		EXESUFFIX=.exe
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
	if ! git diff-index --quiet HEAD . ; then
	echo "Warning: static change and not commited"
	exit 1
	fi
	sh generate.sh
	popd
fi

pushd $__dir/..
BINFILE=${BIN}_${OS}_${ARCH}${SUFFIX} 
rm -fv ${BIN}_*
CGO_ENABLED=0 GOARCH=$ARCH GOOS=$OS go build -o ${BINFILE}${EXESUFFIX} -ldflags "-s -w -X main.VERSION=$GITVER"
if [[ ! -f ${BINFILE}${EXESUFFIX} ]]; then
  echo "Build failed. Check with error message above."
  exit 1
fi

git co HEAD -- static/files.go

if [[ ! -z $PKGCMD ]]; then
  ${PKGCMD} -v -9 -k ${BINFILE}${EXESUFFIX}
fi

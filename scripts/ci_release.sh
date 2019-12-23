#!/bin/bash
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__file="${__dir}/$(basename "${BASH_SOURCE[0]}")"
__base="$(basename ${__file} .sh)"
__root="$(cd "$(dirname "${__dir}")" && pwd)" # <-- change this as it depends on your app

BUILDDIR=$__dir/build
BIN=cloud-torrent
mkdir -p $BUILDDIR
GITVER=$(git describe --tags)

makebuild () {
  local PREFIX=$1
  local OS=$2
  local ARCH=$3
  local SUFFIX=
  if [[ ${OS} == "windows" ]]; then
    SUFFIX=".exe"
  fi
  BINFILE=${BIN}_${OS}_${ARCH}${SUFFIX} 

  if [[ ${ARCH} == "arm" ]]; then
    for GM in 5 6 7; do
      SUFFIX="_armv${GM}"
      BINFILE=${BIN}_${OS}_${ARCH}${SUFFIX} 
      CGO_ENABLED=0 GOARCH=$ARCH GOARM=${GM} GOOS=$OS go build -o ${BUILDDIR}/${BINFILE} -ldflags "-s -w -X main.VERSION=$GITVER"
      pushd ${BUILDDIR}
      gzip -v -9 ${BINFILE}
      popd
    done
  else
    CGO_ENABLED=0 GOARCH=$ARCH GOOS=$OS go build -o ${BUILDDIR}/${BINFILE} -ldflags "-s -w -X main.VERSION=$GITVER"
    pushd ${BUILDDIR}
    gzip -v -9 ${BINFILE}
    popd
  fi
}

upstatic () {
  pushd $__dir/../static
  env PATH=$HOME/go/bin:$PATH bash generate.sh
  popd
}

upstatic
makebuild $BIN $1 $2


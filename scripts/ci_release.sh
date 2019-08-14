#!/bin/bash
__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
__file="${__dir}/$(basename "${BASH_SOURCE[0]}")"
__base="$(basename ${__file} .sh)"
__root="$(cd "$(dirname "${__dir}")" && pwd)" # <-- change this as it depends on your app

BUILDDIR=$__dir/build
BIN=cloud-torrent
GITVER=$(git describe --tags)
mkdir -p $BUILDDIR

makebuild () {
  local PREFIX=$1
  local OS=$2
  local ARCH=$3
  local SUFFIX=${4:-}
  BINFILE=${BIN}_${OS}_${ARCH}${SUFFIX} 
  CGO_ENABLED=0 GOARCH=$ARCH GOOS=$OS go build -o ${BUILDDIR}/${BINFILE} -ldflags "-s -w -X main.VERSION=$GITVER"
  git checkout -- .
  pushd ${BUILDDIR}
  gzip -v -9 ${BINFILE}
  popd
}

makebuild $BIN linux amd64
makebuild $BIN linux 386
makebuild $BIN linux arm
makebuild $BIN linux arm64
makebuild $BIN linux mipsle
makebuild $BIN linux mips
makebuild $BIN windows amd64 .exe
makebuild $BIN windows 386 .exe
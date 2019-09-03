#!/bin/bash

__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GITVER=$(git describe --tags)
sed -i "s/CLDVER/${GITVER}/g" ${__dir}/files/index.html
go generate
git checkout -- ${__dir}/files/index.html
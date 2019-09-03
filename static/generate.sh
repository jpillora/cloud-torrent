#!/bin/bash

__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd ${__dir}
GITVER=$(git describe --tags)
sed -i "s/CLDVER/${GITVER}/g" ${__dir}/files/index.html
sed -i "s/CLDVER/${GITVER}/g" ${__dir}/files/template/downloads.html
go generate
git checkout -- ${__dir}/files/index.html ${__dir}/files/template/downloads.html
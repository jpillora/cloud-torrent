#!/bin/bash

__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd ${__dir}
GITVER=$(git describe --tags)
sed -i "s/CLDVER/${GITVER}/g" ${__dir}/files/index.html \
    ${__dir}/files/template/downloads.html \
    ${__dir}/files/css/app.css

go generate
git checkout -- ${__dir}/files/index.html \
    ${__dir}/files/template/downloads.html \
    ${__dir}/files/css/app.css
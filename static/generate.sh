#!/bin/sh
GITVER=$(git describe --tags)
sed -i "s/CLDVER/${GITVER}/g" \
    files/index.html \
    files/template/magadded.html \
    files/template/downloads.html \
    files/css/app.css

go generate
git checkout -- files

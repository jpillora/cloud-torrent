#!/bin/bash
banner="// velox - v0.2.12 - https://github.com/jpillora/velox
// Jaime Pillora <dev@jpillora.com> - MIT Copyright 2016"
echo "create dist"
echo "$banner" > dist/velox.js
echo "(function() {" >> dist/velox.js
cat vendor/*.js *.js >> dist/velox.js
echo "}());" >> dist/velox.js
which uglifyjs > /dev/null || (echo "please 'npm install uglify-js'"; exit 1)
echo "minify dist"
echo "$banner" > dist/velox.min.js
uglifyjs --mangle -c=dead_code dist/velox.js >> dist/velox.min.js || (echo "uglify failed"; exit 1)
echo "embed dist"
go-bindata -pkg assets -o assets.go dist/velox.js dist/velox.min.js || (echo "go-bindata failed"; exit 1)
exit 0

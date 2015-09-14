#!/bin/bash

for i in * ; do
  if [ -d "$i" ] && [ -f "$i/README.md" ]; then
  # if [ -d "$i" ] && [ -f "%i/README.md"]; then
  	cd "$i"
  	echo "$i"
  	md-tmpl README.md || exit 1
  	cd ..
  fi
done
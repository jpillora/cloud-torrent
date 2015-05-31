#!/bin/bash
go-bindata -pkg embed -ignore "\/\." -o ctserver/embed/files.go embed/
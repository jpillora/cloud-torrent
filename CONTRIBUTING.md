### Contributing guide

Quick start:

* Download Go
* Setup `GOPATH`
* `go get github.com/jpillora/cloud-torrent`
* `cd $GOPATH/src/github.com/jpillora/cloud-torrent`
* Fork this repo
* Change remote to your fork
* Edit static files
* Edit backend Go files
* `go generate ./... && go install`
* `$GOPATH/bin/cloud-torrent`

To add new dependencies:

* `cd $GOPATH/src/github.com/jpillora/cloud-torrent`
* `rm -rf vendor/`
* `go get -u -v .` Fetch all dependencies into your GOPATH
* `go get github.com/foo/bar`
* `godep save && rm -rf Godep/`
* Now `vendor/` should contain the latest deps for all packages
* Test with `go install -v` and you should see the build targeting packages within `vendor/...`
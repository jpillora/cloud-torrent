![screenshot](https://user-images.githubusercontent.com/1033514/62452213-4fa04800-b7a2-11e9-887b-e0e436c1c204.png)

**Simple Torrent** is a a self-hosted remote torrent client, written in Go (golang). You start torrents remotely, which are downloaded as sets of files on the local disk of the server, which are then retrievable or streamable via HTTP.

This is a fork of [Cloud Torrent](https://github.com/jpillora/cloud-torrent).

### New Features

This fork adds new features to the original version by `jpillora`.

* Run extrenal program on task/file completed: `DoneCmd`
* Stops task when seeding ratio reached: `SeedRatio`
* Download/Upload Rate limit: `UploadRate`/`DownloadRate`
* Detailed transfer stats in web UI.

And some development improvement
* Go modules introduced and compatiable with go 1.12
* Upgrade torrnet engine API from github.com/anacrolix/torrent

This fork use version number above 1.0.0.

Inherited features:

* Single binary
* Cross platform
* Embedded torrent search
* Real-time updates
* Mobile-friendly
* Fast [content server](http://golang.org/pkg/net/http/#ServeContent)

### Install

**Binaries**

See [the latest release](https://github.com/boypt/cloud-torrent/releases/latest) or download and install it now.

*Tip*: [Auto-run `cloud-torrent` on boot](https://github.com/jpillora/cloud-torrent/wiki/Auto-Run-on-Reboot)

**Docker**

[![Docker Pulls](https://img.shields.io/docker/pulls/boypt/cloud-torrent.svg)][dockerhub] [![Image Size](https://images.microbadger.com/badges/image/boypt/cloud-torrent.svg)][dockerhub]

[dockerhub]: https://hub.docker.com/r/boypt/cloud-torrent/

``` sh
$ docker run -d -p 3000:3000 -v /path/to/my/downloads:/downloads boypt/cloud-torrent
```

**Source**

*[Go](https://golang.org/dl/) is required to install from source*

``` sh
$ git clone https://github.com/boypt/cloud-torrent.git
$ cd cloud-torrent
$ go get -v ./...
$ CGO_ENABLED=0 go build -o cloud-torrent -ldflags "-s -w -X main.VERSION=1.X.Y"
# or simplly run `make_release.sh'
```

### Usage

```
$ cloud-torrent --help

  Usage: cloud-torrent_linux_amd64 [options]

  Options:
  --title, -t        Title of this instance (default Simple Torrent)
  --port, -p         Listening port (default 3000)
  --host, -h         Listening interface (default all)
  --auth, -a         Optional basic auth in form 'user:password'
  --config-path, -c  Configuration file path (default cloud-torrent.json)
  --key-path, -k     TLS Key file path
  --cert-path        TLS Certicate file path
  --log, -l          Enable request logging
  --open, -o         Open now with your default browser
  --version, -v      display version
  --help             display help

  Version:
    1.0.4-1-g7143c86

  Read more:
    https://github.com/boypt/simple-torrent

```

#### Example of cloud-torrent.json

A sample json will be generated on first run of cloud-torrent.

```json
{
  "AutoStart": true,
  "DisableEncryption": false,
  "DownloadDirectory": "/home/ubuntu/Download/cloud-torrent/downloads",
  "EnableUpload": true,
  "EnableSeeding": false,
  "IncomingPort": 50007,
  "SeedRatio": 1.0,
  "UploadRate": "High",
  "DownloadRate": "High",
  "DoneCmd": ""
}
```

Note: About `UploadRate`/`DownloadRate`, a fixed level amoung `Low`, `Medium` and `High` is accepted as value, all other values(or empty) will result in unlimited rate. The actual rate of each level:

* Low: 50000 Bytes/s (~50k/s)
* Medium: 500000 Bytes/s (~500k/s)
* High: 1500000 Bytes/s (~1500k/s)

#### About DoneCmd

`DoneCmd` is an external program to be called when a torrent task is finished.

```
CLD_DIR=/path/of/DownloadDirectory
CLD_PATH=Torrent-Downloaded-File-OR-Dir
CLD_SIZE=46578901
CLD_TYPE=torrent|file
```

`DoneCmd` will be called at least twice (multiple times if the torrent contains more than one file), one with `CLD_TYPE=file` when the file is completed, and one when the whole torrent complited, with `CLD_TYPE=torrent`.

### Notes

This project is a fork to [Cloud Torrent](https://github.com/jpillora/cloud-torrent).

Credits to @anacrolix for https://github.com/anacrolix/torrent

Copyright (c) 2019 Ben
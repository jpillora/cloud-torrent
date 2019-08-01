![screenshot](https://user-images.githubusercontent.com/1033514/62262813-a4635c00-b44c-11e9-90d3-47ae6f148bbd.png)

**Cloud torrent** is a a self-hosted remote torrent client, written in Go (golang). You start torrents remotely, which are downloaded as sets of files on the local disk of the server, which are then retrievable or streamable via HTTP.

### New Features

This fork adds new features to the original version by jpillora.

* Run extrenal program on task completed: `DoneCmd`
* Stops task when seeding ratio reaching: `SeedRatio`
* Download/Upload Rate limit
* Display transfer stats in web

And some development improvement
* Use go modules with go 1.12
* Upgrade torrnet engine API from github.com/anacrolix/torrent

This fork use version number above 1.0.0.

### Features

* Single binary
* Cross platform
* Embedded torrent search
* Real-time updates
* Mobile-friendly
* Fast [content server](http://golang.org/pkg/net/http/#ServeContent)
* External command run on task finished.

### Install

**Binaries**

See [the latest release](https://github.com/boypt/cloud-torrent/releases/latest) or download and install it now with

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


  Usage: cloud-torrent [options]

  Options:
  --title, -t        Title of this instance (default Cloud Torrent, env TITLE)
  --port, -p         Listening port (default 3000, env PORT)
  --host, -h         Listening interface (default all)
  --auth, -a         Optional basic auth in form 'user:password' (env AUTH)
  --config-path, -c  Configuration file path (default cloud-torrent.json)
  --key-path, -k     TLS Key file path
  --cert-path, -r    TLS Certicate file path
  --log, -l          Enable request logging
  --open, -o         Open now with your default browser
  --done-cmd, -d     External cmd to run when task completed, environment variables CLD_DIR / CLD_PATH
                     / CLD_SIZE / CLD_FILECNT are set.
  --help
  --version, -v

  Version:
    1.X.Y

  Read more:
    https://github.com/jpillora/cloud-torrent


```

#### Example of config.json

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

Note: about `UploadRate`/`DownloadRate`, a fixed level amoung `Low`, `Medium` and `High` is accepted as value, all other values(or empty) will result in unlimited rate. The actual rate of each level:

* Low: 50000 Bytes/s (~ 50k/s)
* Medium: 500000 Bytes/s (~ 500k/s)
* High: 1500000 Bytes/s (~ 1500k/s)

#### About DoneCmd

`DoneCmd` is an external command to be called when a torrent task is finished or one of the files is completed, with following environment variables:

`DoneCmd` maybe called multiple times if the torrent contains multiple files, each would be called when one of the files is completed(with `CLD_TYPE=file`), and when whole torrent is complited (with `CLD_TYPE=torrent`).

```
CLD_DIR=/path/of/DownloadDirectory
CLD_PATH=Torrent-Downloaded-File-OR-Dir
CLD_SIZE=46578901
CLD_TYPE=torrent|file
```

#### Donate

(This Donate goes to original author jpillora)

If you'd like to buy me a coffee or more, you can donate via [PayPal](https://www.paypal.com/cgi-bin/webscr?cmd=_xclick&business=dev%40jpillora%2ecom&lc=AU&item_name=Open%20Source%20Donation&button_subtype=services&currency_code=USD&bn=PP%2dBuyNowBF%3abtn_buynowCC_LG%2egif%3aNonHosted) or BitCoin `1AxEWoz121JSC3rV8e9MkaN9GAc5Jxvs4`.

### Notes

This project is the rewrite of the original [Node version](https://github.com/jpillora/node-torrent-cloud).

![overview](https://docs.google.com/drawings/d/1ekyeGiehwQRyi6YfFA4_tQaaEpUaS8qihwJ-s3FT_VU/pub?w=606&h=305)

Credits to @anacrolix for https://github.com/anacrolix/torrent

Copyright (c) 2017 Jaime Pillora
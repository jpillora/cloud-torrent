![screenshot](https://user-images.githubusercontent.com/1033514/62452213-4fa04800-b7a2-11e9-887b-e0e436c1c204.png)

**Simple Torrent** is a a self-hosted remote torrent client, written in Go (golang). You start torrents remotely, which are downloaded as sets of files on the local disk of the server, which are then retrievable or streamable via HTTP.

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

Other features:

* Single binary
* Cross platform
* Embedded torrent search
* Real-time updates
* Mobile-friendly
* Fast [content server](http://golang.org/pkg/net/http/#ServeContent)

### Install

**Binaries**

See [the latest release](https://github.com/boypt/cloud-torrent/releases/latest) or use the script to do a quick install on modern Linux.

```
bash <(wget -qO- https://raw.githubusercontent.com/boypt/simple-torrent/master/scripts/quickinstall.sh)
```

NOTE: the script installs a systemd unit at `/etc/systemd/system/cloud-torrent.service`, and runs simple-torrent with authentication `user:ctorrent`, DO EDIT THIS FILE after confirming the program is running correctly.

**Docker**

[![Docker Pulls](https://img.shields.io/docker/pulls/boypt/cloud-torrent.svg)][dockerhub] [![Image Size](https://images.microbadger.com/badges/image/boypt/cloud-torrent.svg)][dockerhub]

[dockerhub]: https://hub.docker.com/r/boypt/cloud-torrent/

``` sh
$ docker run -d -p 3000:3000 -v /path/to/my/downloads:/downloads boypt/cloud-torrent
```

**Source**

*[Go](https://golang.org/dl/) is required to install from source*

``` sh
$ git clone https://github.com/boypt/simple-torrent.git
$ cd simple-torrent
$ ./scripts/make_release.sh
```

### Usage

```
$ cloud-torrent --help

  Usage: cloud-torrent [options]

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
    1.X.Y

  Read more:
    https://github.com/boypt/simple-torrent

```

#### Example of cloud-torrent.json

A sample json will be generated on first run of simple-torrent.

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

`DoneCmd` will be called at least twice (multiple times if the torrent contains more than one file), once with `CLD_TYPE=file` when the file is completed, and when the whole torrent complited, with `CLD_TYPE=torrent`.

### Notes

This project is a fork to [Cloud Torrent](https://github.com/jpillora/cloud-torrent).

Credits to @anacrolix for https://github.com/anacrolix/torrent

Copyright (c) 2019 Ben
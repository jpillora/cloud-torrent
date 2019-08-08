# Simple Torrent
![screenshot](https://user-images.githubusercontent.com/1033514/62452213-4fa04800-b7a2-11e9-887b-e0e436c1c204.png)

**Simple Torrent** is a a self-hosted remote torrent client, written in Go (golang). You start torrents remotely, which are downloaded as sets of files on the local disk of the server, which are then retrievable or streamable via HTTP.

# Features

This fork adds new features to the original version by `jpillora`.

* Run extrenal program on completed: `DoneCmd`
* Stops task when seeding ratio reached: `SeedRatio`
* Download/Upload speed limiter: `UploadRate`/`DownloadRate`
* Detailed transfer stats in web UI.
* Torrent Watcher.

And some development improvement:
* Go modules introduced and compatiable with go 1.12+
* Upgraded torrnet engine API from github.com/anacrolix/torrent

Other features:
* Single binary
* Cross platform
* Embedded torrent search
* Real-time updates
* Mobile-friendly
* Fast [content server](http://golang.org/pkg/net/http/#ServeContent)
* IPv6 out of the box

# Install

## Binaries

See [the latest release](https://github.com/boypt/cloud-torrent/releases/latest) or use the oneline script to do a quick install on modern Linux.

```
bash <(wget -qO- https://raw.githubusercontent.com/boypt/simple-torrent/master/scripts/quickinstall.sh)
```

NOTE: [MUST read wiki page for further intructions: Auth And Security](https://github.com/boypt/simple-torrent/wiki/AuthSecurity)

## Docker

[![Docker Pulls](https://img.shields.io/docker/pulls/boypt/cloud-torrent.svg)][dockerhub] [![Image Size](https://images.microbadger.com/badges/image/boypt/cloud-torrent.svg)][dockerhub]

[dockerhub]: https://hub.docker.com/r/boypt/cloud-torrent/

``` sh
$ docker run -d -p 3000:3000 -v /path/to/my/downloads:/downloads -v /path/to/my/torrents:/torrents boypt/cloud-torrent
```

## Source

*[Go](https://golang.org/dl/) is required to install from source*

``` sh
$ git clone https://github.com/boypt/simple-torrent.git
$ cd simple-torrent
$ ./scripts/make_release.sh
```

# Usage

## Commandline Options
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

## Configuration file

A sample json will be created on the first run of simple-torrent.

```json
{
  "AutoStart": true,
  "DisableEncryption": false,
  "DownloadDirectory": "/home/ubuntu/cloud-torrent/downloads",
  "WatchDirectory": "/home/ubuntu/cloud-torrent/torrents",
  "EnableUpload": true,
  "EnableSeeding": true,
  "IncomingPort": 50007,
  "DoneCmd": "",
  "SeedRatio": 1.5,
  "UploadRate": "High",
  "DownloadRate": "Unlimited"
}
```

* `AutoStart` Whether start torrent task on added Magnet/Torrent.
* `DisableEncryption` A switch disables [BitTorrent protocol encryption](https://en.wikipedia.org/wiki/BitTorrent_protocol_encryption)
* `DownloadDirectory` The Directory where downloaded file saves.
* `WatchDirectory` The Directory simple-torrent will watch, automaticly adds task when `.torrent` files put in.
* `EnableUpload` Whether send chunks to peers
* `EnableSeeding` Whether upload even after there's nothing further for us. By default uploading is not altruistic, we'll only upload to encourage the peer to reciprocate.
* `IncomingPort` The port SimpleTorrent listens to.
* `DoneCmd` An external program to call on task finished. See [DoneCmd Usage](https://github.com/boypt/simple-torrent/wiki/DoneCmdUsage).
* `SeedRatio` The ratio of task Upload/Download data when reached, the task will be stop.
* `UploadRate`/`DownloadRate` The global speed limiter, a fixed level amoung `Low`, `Medium` and `High` is accepted as value, all other values(or empty) will result in unlimited rate. The actual rate of each level:
    Low: 50000 Bytes/s (~50k/s)
    Medium: 500000 Bytes/s (~500k/s)
    High: 1500000 Bytes/s (~1500k/s)


# Credits 
* Credits to @jpillora for [Cloud Torrent](https://github.com/jpillora/cloud-torrent).
* Credits to @anacrolix for https://github.com/anacrolix/torrent

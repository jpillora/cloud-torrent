# SimpleTorrent [![Build Status](https://travis-ci.org/boypt/simple-torrent.svg?branch=master)](https://travis-ci.org/boypt/simple-torrent) 
![screenshot](https://user-images.githubusercontent.com/1033514/64141503-d6e0ea00-ce3a-11e9-9369-10fb7c56aa18.png)

**SimpleTorrent** is a a self-hosted remote torrent client, written in Go (golang). Started torrents remotely, download sets of files on the local disk of the server, which are then retrievable or streamable via HTTP.

# Features

This fork adds new features to the original cloud-torrent by `jpillora`.

* Run extrenal program on tasks completed: `DoneCmd`
* Stops task when seeding ratio reached: `SeedRatio`
* Download/Upload speed limiter: `UploadRate`/`DownloadRate`
* Detailed transfer stats in web UI.
* [Torrent Watcher](https://github.com/boypt/simple-torrent/wiki/Torrent-Watcher)
* K8s/docker health-check endpoint `/healthz`
* Extra trackers add from http source
* Protocol Handler to `magnet:`

And some development improvement:
* Go modules introduced and compatiable with go 1.12+
* Updated and compatiable with torrnet engine API from [anacrolix/torrent](https://github.com/anacrolix/torrent)

Also:
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

## Docker [![Docker Pulls](https://img.shields.io/docker/pulls/boypt/cloud-torrent.svg)][dockerhub] [![Image Size](https://images.microbadger.com/badges/image/boypt/cloud-torrent.svg)][dockerhub]

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

  Usage: cloud-torrent_linux_amd64 [options]

  Options:
  --title, -t             Title of this instance (default SimpleTorrent)
  --port, -p              Listening port (default 3000)
  --host, -h              Listening interface (default all)
  --auth, -a              Optional basic auth in form 'user:password'
  --config-path, -c       Configuration file path (default cloud-torrent.json)
  --key-path, -k          TLS Key file path
  --cert-path             TLS Certicate file path
  --log, -l               Enable request logging
  --open, -o              Open now with your default browser
  --disable-log-time, -d  Don't print timestamp in log
  --version, -v           display version
  --help                  display help

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
  "Debug": false,
  "ObfsPreferred": true,
  "ObfsRequirePreferred": false,
  "DisableTrackers": false,
  "DisableIPv6": false,
  "DownloadDirectory": "/home/ubuntu/Workdir/cloud-torrent/downloads",
  "WatchDirectory": "/home/ubuntu/Workdir/cloud-torrent/torrents",
  "EnableUpload": true,
  "EnableSeeding": true,
  "IncomingPort": 50007,
  "DoneCmd": "",
  "SeedRatio": 1.5,
  "UploadRate": "High",
  "DownloadRate": "Unlimited",
  "TrackerListURL": "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_best.txt"
}
```

* `AutoStart`: Whether start torrent task on added Magnet/Torrent.
* `Debug` Print debug log from torrent engine (lots of them)
* `ObfsPreferred`: Whether torrent header obfuscation is preferred.
* `ObfsRequirePreferred`: Whether the value of `ObfsPreferred` is a strict requirement. This hides torrent traffic from being censored.
* `DisableTrackers`: Don't announce to trackers. This only leaves DHT to discover peers.
* `DisableIPv6`: Don't connect to IPv6 peers.
* `DisableEncryption` A switch disables [BitTorrent protocol encryption](https://en.wikipedia.org/wiki/BitTorrent_protocol_encryption)
* `DownloadDirectory` The directory where downloaded file saves.
* `WatchDirectory` The directory SimpleTorrent will watch and load new added `.torrent`, See [Torrent Watcher](https://github.com/boypt/simple-torrent/wiki/Torrent-Watcher)
* `EnableUpload` Whether send chunks to peers
* `EnableSeeding` Whether upload even after there's nothing further for us. By default uploading is not altruistic, we'll only upload to encourage the peer to reciprocate.
* `IncomingPort` The port SimpleTorrent listens to.
* `DoneCmd` An external program to call on task finished. See [DoneCmd Usage](https://github.com/boypt/simple-torrent/wiki/DoneCmdUsage).
* `SeedRatio` The ratio of task Upload/Download data when reached, the task will be stop.
* `UploadRate`/`DownloadRate` The global speed limiter, a fixed level amoung `Low`(~50k/s), `Medium`(~500k/s) and `High`(~1500k/s) is accepted as value, all other values (or empty) will result in unlimited rate.
* `TrackerListURL`: A https URL to a trackers list, this option is design to retrive public trackers from [ngosang/trackerslist](https://github.com/ngosang/trackerslist). If configred, all trackers will be added to each torrent task.


# Credits 
* Credits to @jpillora for [Cloud Torrent](https://github.com/jpillora/cloud-torrent).
* Credits to @anacrolix for https://github.com/anacrolix/torrent

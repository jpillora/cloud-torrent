![screenshot](https://user-images.githubusercontent.com/1033514/64239393-bdbb6480-cf32-11e9-9269-d8d10e7c0dc7.png)

[![Build Status](https://travis-ci.org/boypt/simple-torrent.svg?branch=master)](https://travis-ci.org/boypt/simple-torrent) 
**SimpleTorrent** is a a self-hosted remote torrent client, written in Go (golang). Started torrents remotely, download sets of files on the local disk of the server, which are then retrievable or streamable via HTTP.

This project is a re-branded fork of cloud-torrent by `jpillora`.

# Features

* Run extrenal program on tasks completed: `DoneCmd`
* Stops task when seeding ratio reached: `SeedRatio`
* Download/Upload speed limiter: `UploadRate`/`DownloadRate`
* Detailed transfer stats in web UI.
* [Torrent Watcher](https://github.com/boypt/simple-torrent/wiki/Torrent-Watcher)
* K8s/docker health-check endpoint `/healthz`
* Extra trackers add from http source
* Protocol Handler to `magnet:`

Also:
* Single binary
* Cross platform
* Embedded torrent search
* Real-time updates
* Mobile-friendly
* Fast [content server](http://golang.org/pkg/net/http/#ServeContent)
* IPv6 out of the box
* Go modules introduced and compatiable with go 1.12+
* Updated torrnet engine from [anacrolix/torrent](https://github.com/anacrolix/torrent)

# Install

## Binary

See [the latest release](https://github.com/boypt/cloud-torrent/releases/latest) or use the oneline script to do a quick install on modern Linux.

```
bash <(wget -qO- https://raw.githubusercontent.com/boypt/simple-torrent/master/scripts/quickinstall.sh)
```

The script install a systemd unit (under `scripts/cloud-torrent.service`) as service. Read further intructions: [Auth And Security](https://github.com/boypt/simple-torrent/wiki/AuthSecurity)

## Docker [![Docker Pulls](https://img.shields.io/docker/pulls/boypt/cloud-torrent.svg)][dockerhub] [![Image Size](https://images.microbadger.com/badges/image/boypt/cloud-torrent.svg)][dockerhub]

[dockerhub]: https://hub.docker.com/r/boypt/cloud-torrent/

``` sh
$ docker run -d -p 3000:3000 -v /path/to/my/downloads:/downloads -v /path/to/my/torrents:/torrents boypt/cloud-torrent
```

## Source

*[Golang](https://golang.org/dl/) is required to compile from source*

``` sh
$ git clone https://github.com/boypt/simple-torrent.git
$ cd simple-torrent
$ ./scripts/make_release.sh
```

# Usage

## Commandline Options
See Wiki [Command line Options](https://github.com/boypt/simple-torrent/wiki/Command-line-Options)

## Configuration file
See Wiki [Detailed Explanation of `config.json`](https://github.com/boypt/simple-torrent/wiki/Detailed-Explanation-of-config.json)

## Use with WEB servers (nginx/caddy)
See Wiki [Behind WebServer (reverse proxying)](https://github.com/boypt/simple-torrent/wiki/ReverseProxy)

# Credits 
* Credits to @jpillora for [Cloud Torrent](https://github.com/jpillora/cloud-torrent).
* Credits to @anacrolix for https://github.com/anacrolix/torrent

<img src="https://cloud.githubusercontent.com/assets/633843/9855504/f30a715c-5b51-11e5-83f3-f4fab03e5459.png" alt="screenshot"/>

**Cloud torrent** is a a self-hosted remote torrent client, written in Go (golang). You start torrents remotely, which are downloaded as sets of files on the local disk of the server, which are then retrievable or streamable via HTTP.

### Features

* Single binary
* Cross platform
* Embedded torrent search
* Real-time updates
* Mobile-friendly
* Fast [content server](http://golang.org/pkg/net/http/#ServeContent)
* [*More features coming soon*](https://github.com/jpillora/cloud-torrent/labels/core-feature)

### Install

**Binaries**

See [the latest release](https://github.com/jpillora/cloud-torrent/releases/latest) or download it now with `curl i.jpillora.com/cloud-torrent | bash`

**Source**

*[Go](https://golang.org/dl/) is required to install from source*

``` sh
$ go get -v github.com/jpillora/cloud-torrent
```

**Docker**

``` sh
$ docker run -d -p 3000:3000 -v /path/to/my/downloads:/downloads jpillora/cloud-torrent
```

**Heroku**

Click this button to...

[![Deploy](https://www.herokucdn.com/deploy/button.png)](https://heroku.com/deploy)

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
  --help
  --version, -v

  Version:
    0.8.8

  Read more:
    https://github.com/jpillora/cloud-torrent

```

### Notes

This project is the version 2 rewrite of the original [Node version](https://github.com/jpillora/node-torrent-cloud).

![overview](https://docs.google.com/drawings/d/1ekyeGiehwQRyi6YfFA4_tQaaEpUaS8qihwJ-s3FT_VU/pub?w=606&h=305)

Credits to `anacrolix` for https://github.com/anacrolix/torrent

#### License

Copyright (c) 2016 Jaime Pillora

[Creative Commons Legal Code - Attribution-NonCommercial 3.0 Unported](LICENSE)

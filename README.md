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

:warning: Currently, `0.8.12` only has linux 64bit releases. Please either compile from source or download a `0.8.11` release.

**Source**

*[Go](https://golang.org/dl/) is required to install from source*

``` sh
$ go get -v github.com/jpillora/cloud-torrent
```

**Docker**

[![Docker Pulls](https://img.shields.io/docker/pulls/jpillora/cloud-torrent.svg)][dockerhub] [![Image Size](https://images.microbadger.com/badges/image/jpillora/cloud-torrent.svg)][dockerhub]

[dockerhub]: https://hub.docker.com/r/jpillora/cloud-torrent/

``` sh
$ docker run -d -p 3000:3000 -v /path/to/my/downloads:/downloads jpillora/cloud-torrent
```

**VPS**

[Digital Ocean](https://m.do.co/c/011fa87fde07)

  1. [Sign up with free $10 credit](https://m.do.co/c/011fa87fde07)
  2. "Create Droplet"
  3. "One-Click Apps"
  4. "Docker X.X.X on X.X"
  5. Choose server size ("$5/month" is enough)
  6. Choose server location
  7. **OPTIONAL** Add your SSH key
  8. "Create"
  9. You will be emailed the server details (`IP Address: ..., Username: root, Password: ...`)
  10. SSH into the server using these details (Windows: [Putty](https://the.earth.li/~sgtatham/putty/latest/x86/putty.exe), Mac: Terminal)
  11. Follow the prompts to set a new password
  12. Run `cloud-torrent` with:

    ```
    docker run --name ct -d -p 63000:63000 \
      -v /etc/ssl/certs/ca-certificates.crt:/etc/ssl/certs/ca-certificates.crt \
      -v /root/downloads:/downloads \
      jpillora/cloud-torrent --port 63000
    ```

  13. Visit `http://<IP Address from email>:63000/`
  14. **OPTIONAL** In addition to `--port` you can specify the options below

[Vultr](http://www.vultr.com/?ref=6947403-3B)

* [Sign up with free $30 credit here](http://www.vultr.com/?ref=6947403-3B)
* Follow the DO tutorial above, very similar steps ("Applications" instead of "One-Click Apps")
* Offers different server locations

[AWS](https://aws.amazon.com)

**Heroku**

Heroku is no longer supported

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
    0.8.11

  Read more:
    https://github.com/jpillora/cloud-torrent

```

### Notes

This project is the version 2 rewrite of the original [Node version](https://github.com/jpillora/node-torrent-cloud).

![overview](https://docs.google.com/drawings/d/1ekyeGiehwQRyi6YfFA4_tQaaEpUaS8qihwJ-s3FT_VU/pub?w=606&h=305)

Credits to @anacrolix for https://github.com/anacrolix/torrent

#### License

Copyright (c) 2016 Jaime Pillora

[Creative Commons Legal Code - Attribution-NonCommercial 3.0 Unported](LICENSE)

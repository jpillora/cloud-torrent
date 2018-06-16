<div align="center">

# cloud-torrent

:sun_behind_large_cloud: Self-hosted remote torrent client

[![Releases][shield-total-dl]][link-release]
[![Latest Release][shield-release]][link-release-latest]
[![Docker Pulls][shield-docker-pulls]][link-docker]
[![Image Size][shield-docker-size]][link-docker]
[![Discord][shield-discord]](https://discord.gg/x4sa3fP)

[![cloud-torrent](https://griko.keybase.pub/shared/screenshots/cloud-torrent-preview.png)](.)

</div>

**cloud-torrent** is a self-hosted remote torrent client written in
[golang](https://golang.org). You start torrents remotely, which are downloaded
as sets of files on the local disk of the server, which are then retrievable or
streamable via HTTP.

## Features

- Single binary
- Cross-platform (Windows, Linux, macOS, or anything that runs Go)
- Embedded torrent search
- Real-time updates
- Mobile-friendly
- Fast content server ([read more](http://golang.org/pkg/net/http/#ServeContent))

More features coming soon, see [upcoming features](#upcoming-features) for more
information.

## Architecture

![Architecture][image-arch]

Fun fact: **cloud-torrent** was originally developed using
[Node.js](https://github.com/jpillora/node-torrent-cloud).

## How to use

### Using binaries

Download the [latest release on GitHub][link-release-latest] or run this
command to automatically install **cloud-torrent**:

```sh
curl https://i.jpillora.com/cloud-torrent! | bash
```

Here's how to [configure **cloud-torrent** to run on boot][wiki-autorun].

### Using Docker

**cloud-torrent** is also avaliable as a [Docker image][link-docker], which you
can run by running this command:

```sh
docker run -d -p 3000:3000 \
        -v /path/to/my/downloads:/downloads \
        jpillora/cloud-torrent
```

### Build from sources

Install [golang](https://golang.org/dl) on your machine and run this command:

```sh
go get -v github.com/jpillora/cloud-torrent
```

## Usage

You can type `--help` to view all available options:

```text
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
    0.X.Y

  Read more:
    https://github.com/jpillora/cloud-torrent
```

## Contributing

### Report issues

Submit your issue by describing your problem thoroughly, include screenshots
and steps to reproduce the current issue. Duplicate or undetailed issues will
be closed. For quick questions, please join the
[Discord community server](https://discord.gg/x4sa3fP).

### Help with development

1. Fork this project on GitHub
2. Do your magic
3. Submit a [pull request](https://github.com/jpillora/cloud-torrent/compare)
4. ...
5. Profit!

See the [contributing readme][link-contribute] for more information.

## Upcoming features

The next set of [core features can be tracked here][link-upcoming]. Most
features requires large structural changes and therefore requires a complete
rewrite for best results. The rewrite is currently in progress in the `v0.9`
branch, though it will take quite some time.

In summary, here are the upcoming features:

- **Remote backends**

  Version `0.9` will be more of a general purpose cloud transfer engine, which
  will be capable of transfering files from and to any filesystem. A torrent
  can be viewed as a folder with files, just like your local disk and Dropbox.
  As long as it has a concept of files and folders, it could potentially be a
  **cloud-torrent** filesystem backend. You can
  [track this issue (#24)][link-issue-24] and view the list of proposed
  backends.

- **File Transforms**

  During a file transfer, one could apply different transforms against the byte
  stream for various effects. For example, supported transforms might include
  video transcoding (using `ffmpeg`), encryption and decryption,
  [media sorting or file renaming (#4)][link-issue-4], and writing multiple
  files as a single zip file.

- **Automatic updates**

  Binary will upgrade itself, which adds new features as they get released
  without having to manually download new versions.

- **RSS Feeds**

  Automatically add torrents based on RSS, with smart episode filters for
  downloading movies or TV shows.

## Donate

Your donations helps [@jpillora](https://github.com/jpillora) supercharge his
development. You can donate to via [Paypal][link-donate-paypal] or Bitcoin
`1AxEWoz121JSC3rV8e9MkaN9GAc5Jxvs4`.

## Special thanks

- [anacrolix/torrent](https://github.com/anacrolix/torrent) - BitTorrent
  package for golang

[image-arch]: https://docs.google.com/drawings/d/1ekyeGiehwQRyi6YfFA4_tQaaEpUaS8qihwJ-s3FT_VU/pub?w=606&h=305

[shield-discord]: https://img.shields.io/discord/457548371633373186.svg
[shield-docker-pulls]: https://img.shields.io/docker/pulls/jpillora/cloud-torrent.svg
[shield-docker-size]: https://images.microbadger.com/badges/image/jpillora/cloud-torrent.svg
[shield-release]: https://img.shields.io/github/release/jpillora/cloud-torrent.svg
[shield-total-dl]: https://img.shields.io/github/downloads/jpillora/cloud-torrent/total.svg

[link-contribute]: https://github.com/jpillora/cloud-torrent/blob/master/CONTRIBUTING.md
[link-docker]: https://hub.docker.com/r/jpillora/cloud-torrent
[link-donate-paypal]: https://www.paypal.com/cgi-bin/webscr?cmd=_xclick&business=dev%40jpillora%2ecom&lc=AU&item_name=Open%20Source%20Donation&button_subtype=services&currency_code=USD&bn=PP%2dBuyNowBF%3abtn_buynowCC_LG%2egif%3aNonHosted
[link-issue-4]: https://github.com/jpillora/cloud-torrent/issues/4
[link-issue-24]: https://github.com/jpillora/cloud-torrent/issues/24
[link-release]: https://github.com/jpillora/cloud-torrent/releases
[link-release-latest]: https://github.com/jpillora/cloud-torrent/releases/latest
[link-upcoming]: https://github.com/jpillora/cloud-torrent/issues?q=is%3Aopen+is%3Aissue+label%3Acore-feature

[wiki-autorun]: https://github.com/jpillora/cloud-torrent/wiki/Auto-Run-on-Reboot

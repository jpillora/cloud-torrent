#!/usr/bin/env bash
# Bash3 Boilerplate. Copyright (c) 2014, kvz.io

set -o errexit
set -o pipefail
set -o nounset

if ! command -v systemctl >/dev/null 2>&1; then
    echo "> Sorry but this scripts is only for Linux with systemd, eg: Ubuntu 16.04+/Centos 7+ ..."
    exit 1
fi
HOSTIP=""
for IP in $(hostname --all-ip-address); do
  if [[ $IP == ::* ]]; then continue; fi
  if [[ $IP == fe80* ]]; then continue; fi
  if [[ $IP == 127* ]]; then continue; fi
  HOSTIP=$IP
  break
done

CLDBIN=/usr/local/bin/cloud-torrent
OSARCH=$(uname -m)
case $OSARCH in 
    x86_64)
        BINTAG=linux_amd64
        ;;
    i*86)
        BINTAG=linux_386
        ;;
    arm64)
        BINTAG=linux_arm64
        ;;
    arm*)
        BINTAG=linux_arm
        ;;
    *)
        echo "unsupported OSARCH: $OSARCH"
        exit 1
        ;;
esac

systemctl stop cloud-torrent || true
wget -qO- https://api.github.com/repos/boypt/simple-torrent/releases/latest \
| grep browser_download_url | grep "$BINTAG" | cut -d '"' -f 4 \
| wget --no-verbose -i- -O- | gzip -d -c > ${CLDBIN}
chmod 0755 ${CLDBIN}

wget -O /etc/systemd/system/cloud-torrent.service https://raw.githubusercontent.com/boypt/simple-torrent/master/scripts/cloud-torrent.service
systemctl daemon-reload
systemctl start cloud-torrent
systemctl enable cloud-torrent

cat <<EOF
#################################################################
              SimpleTorrent installed successfuly.

Open browser to http://${HOSTIP}:3000/ now!

* Default DownloadDirectory: /root/downlods
* Default Config file: /root/cloud-torrent.json
* Default Username: user
* Default Password: ctorrent

Read the wiki page about changing the default settings.
    https://github.com/boypt/simple-torrent/wiki/AuthSecurity

#################################################################
EOF
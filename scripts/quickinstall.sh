#!/usr/bin/env bash
# Bash3 Boilerplate. Copyright (c) 2014, kvz.io

set -o errexit
set -o pipefail
set -o nounset

if ! command -v systemctl >/dev/null 2>&1; then
    echo "> Sorry but this scripts is only for Linux with systemd, eg: Ubuntu 16.04+/Centos 7+ ..."
    exit 1
fi

if [[ $(id -u) -ne 0 ]]; then
    echo "This script must be run as root" 
    exit 1
fi

HOSTIP=$(ip -o route get to 8.8.8.8 | sed -n 's/.*src \([0-9.]\+\).*/\1/p')
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

read -p "Need authentication? (Y/N)" NEEDAUTH
USERNAME="(none)"
PASSWORD="(none)"
if [[ x${NEEDAUTH^^} == x"Y" ]]; then
    read -p "Input Username:" USERNAME
    read -p "Input Password:" PASSWORD
fi

systemctl stop cloud-torrent || true
wget -qO- https://api.github.com/repos/boypt/simple-torrent/releases/latest \
| grep browser_download_url | grep "$BINTAG" | cut -d '"' -f 4 \
| wget --no-verbose -i- -O- | gzip -d -c > ${CLDBIN}
chmod 0755 ${CLDBIN}

wget -O /etc/systemd/system/cloud-torrent.service https://raw.githubusercontent.com/boypt/simple-torrent/master/scripts/cloud-torrent.service

if [[ x${NEEDAUTH^^} == x"Y" ]]; then
    sed -i "s/user:ctorrent/${USERNAME}:${PASSWORD}/" /etc/systemd/system/cloud-torrent.service 
else
    sed -i "/AUTH/s/^/#/" /etc/systemd/system/cloud-torrent.service 
fi

systemctl daemon-reload
systemctl start cloud-torrent
systemctl enable cloud-torrent

cat <<EOF
#################################################################
              SimpleTorrent installed successfuly.

Open browser to http://${HOSTIP}:3000/ now!

* Default DownloadDirectory: /root/downloads
* Default Config file: /root/cloud-torrent.json
* Default Username: ${USERNAME}
* Default Password: ${PASSWORD}

Read the wiki page about changing the default settings.
    https://github.com/boypt/simple-torrent/wiki/AuthSecurity

#################################################################
EOF

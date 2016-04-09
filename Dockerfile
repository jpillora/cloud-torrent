FROM alpine
MAINTAINER dev@jpillora.com

#configure go path
ENV GOPATH /root/go
ENV PATH $PATH:/usr/local/go/bin:$GOPATH/bin

#package
ENV PACKAGE github.com/jpillora/cloud-torrent
ENV PACKAGE_DIR $GOPATH/src/$PACKAGE

#install go and godep, then compile cloud-torrent using godep, then wipe build tools
RUN apk update && \
    apk add git go gzip && \
    go get github.com/tools/godep && \
    mkdir -p $PACKAGE_DIR && \
    git clone https://$PACKAGE.git $PACKAGE_DIR && \
    cd $PACKAGE_DIR && \
    godep go build -ldflags "-X main.VERSION=$(git describe --abbrev=0 --tags)" -o /usr/local/bin/cloud-torrent && \
    cd /tmp && \
    rm -rf $GOPATH && \
    apk del git go gzip && \
    echo "Installed $PACKAGE"

#run package
ENTRYPOINT ["cloud-torrent"]

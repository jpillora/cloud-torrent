############################
# STEP 1 build executable binary
############################
FROM golang:alpine AS builder
RUN apk update && apk add --no-cache git
WORKDIR /root/cloud-torrent
ENV PATH=$HOME/go/bin:$PATH 
RUN git clone https://github.com/boypt/cloud-torrent.git . && \
    go get -v -u github.com/shuLhan/go-bindata/... && \
    go get -v -t -d ./... && \
    cd static && \
    sh generate.sh

ENV GO111MODULE=on CGO_ENABLED=0
RUN go build -ldflags "-s -w -X main.VERSION=$(git describe --tags)" -o /usr/local/bin/cloud-torrent
############################
# STEP 2 build a small image
############################
FROM alpine
COPY --from=builder /usr/local/bin/cloud-torrent /usr/local/bin/cloud-torrent
RUN apk update && apk add ca-certificates
ENTRYPOINT ["cloud-torrent"]

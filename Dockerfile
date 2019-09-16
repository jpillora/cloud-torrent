############################
# STEP 1 build executable binary
############################
FROM golang:alpine AS builder
RUN apk update && apk add --no-cache git
WORKDIR /root/cloud-torrent
RUN git clone https://github.com/boypt/cloud-torrent.git .
ENV GO111MODULE=on
RUN go build -ldflags "-s -w -X main.VERSION=$(git describe --abbrev=0 --tags)" -o /usr/local/bin/cloud-torrent
############################
# STEP 2 build a small image
############################
FROM alpine
COPY --from=builder /usr/local/bin/cloud-torrent /usr/local/bin/cloud-torrent
RUN apk update && apk add ca-certificates
ENTRYPOINT ["cloud-torrent"]

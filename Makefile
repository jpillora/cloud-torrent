bin=cloud-torrent
.PHONY: clean
all: clean $(bin)

cloud-torrent:
	go build -ldflags "-s -w -X main.VERSION=git-$$(git rev-parse --short HEAD)"

clean:
	rm -fv $(bin)

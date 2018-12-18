bin=cloud-torrent
.PHONY: clean
all: clean $(bin)

cloud-torrent:
	CGO_ENABLED=0 go build -o $(bin) -ldflags "-s -w -X main.VERSION=git-$$(git rev-parse --short HEAD)"

clean:
	rm -fv $(bin)

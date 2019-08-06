bin=cloud-torrent
.PHONY: clean
all: clean $(bin)

cloud-torrent:
	CGO_ENABLED=0 go build -o $(bin) -ldflags "-s -w -X main.VERSION=$$(git describe --tags)"

clean:
	rm -fv $(bin)

app_name = simple-torrent

build: get
	CGO_ENABLED=0 go build -ldflags "-X main.VERSION=`git rev-parse --short HEAD`" -o $(app_name)

static: get
	go build -tags "netgo,osusergo,sqlite_omit_load_extension" -ldflags "-X main.VERSION=`git rev-parse --short HEAD` -linkmode external -extldflags "-static" " -o $(app_name)

get:
	go mod download

run:
	go run .

test:
	go test ./... -v

clean:
	rm -f $(app_name)

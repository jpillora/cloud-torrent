module cloud-torrent

go 1.16

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/StackExchange/wmi v0.0.0-20210224194228-fe8f1750fd46 // indirect
	github.com/anacrolix/dht/v2 v2.9.2-0.20210527235055-9013a10a0c7d // indirect
	github.com/anacrolix/log v0.9.0
	github.com/anacrolix/torrent v1.28.0
	github.com/andrew-d/go-termutil v0.0.0-20150726205930-009166a695a2 // indirect
	github.com/boypt/scraper v1.0.3
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/elithrar/simple-scrypt v1.3.0 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/jpillora/ansi v1.0.2 // indirect
	github.com/jpillora/archive v0.0.0-20160301031048-e0b3681851f1
	github.com/jpillora/cloud-torrent v0.0.0-20180427161701-1a741e3d8dd2
	github.com/jpillora/cookieauth v1.0.0
	github.com/jpillora/opts v1.2.0
	github.com/jpillora/requestlog v1.0.0
	github.com/jpillora/sizestr v1.0.0 // indirect
	github.com/jpillora/velox v0.4.1
	github.com/mmcdole/gofeed v1.1.3
	github.com/shirou/gopsutil v3.21.4+incompatible
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/viper v1.7.1
	github.com/tklauser/go-sysconf v0.3.6 // indirect
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce // indirect
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
)

replace (
	github.com/anacrolix/dht/v2 => github.com/anacrolix/dht/v2 v2.9.2-0.20210527235055-9013a10a0c7d
	github.com/boltdb/bolt => github.com/boypt/bolt v1.3.2
	github.com/jpillora/cloud-torrent => ./
	github.com/jpillora/cloud-torrent/engine => ./engine/
	github.com/jpillora/cloud-torrent/server => ./server/
	github.com/jpillora/cloud-torrent/static => ./static/
	github.com/willf/bitset => github.com/bits-and-blooms/bitset v1.2.0

)

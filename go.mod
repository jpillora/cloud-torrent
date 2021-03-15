module cloud-torrent

go 1.15

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/anacrolix/log v0.8.0
	github.com/anacrolix/torrent v1.25.0
	github.com/andrew-d/go-termutil v0.0.0-20150726205930-009166a695a2 // indirect
	github.com/boypt/scraper v1.0.3
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/elazarl/go-bindata-assetfs v1.0.1
	github.com/elithrar/simple-scrypt v1.3.0 // indirect
	github.com/go-bindata/go-bindata v3.1.2+incompatible // indirect
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/jpillora/ansi v1.0.2 // indirect
	github.com/jpillora/archive v0.0.0-20160301031048-e0b3681851f1
	github.com/jpillora/cloud-torrent v0.0.0-20180427161701-1a741e3d8dd2
	github.com/jpillora/cookieauth v1.0.0
	github.com/jpillora/opts v1.2.0
	github.com/jpillora/requestlog v1.0.0
	github.com/jpillora/sizestr v1.0.0 // indirect
	github.com/jpillora/velox v0.4.0
	github.com/mmcdole/gofeed v1.1.0
	github.com/radovskyb/watcher v1.0.7
	github.com/shirou/gopsutil v3.20.10+incompatible
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/viper v1.7.1
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce // indirect
	golang.org/x/time v0.0.0-20201208040808-7e3f01d25324
)

replace (
	github.com/boltdb/bolt => github.com/boypt/bolt v1.3.2
	github.com/jpillora/cloud-torrent => ./
	github.com/jpillora/cloud-torrent/engine => ./engine/
	github.com/jpillora/cloud-torrent/server => ./server/
	github.com/jpillora/cloud-torrent/static => ./static/
	github.com/jpillora/velox => github.com/boypt/velox v0.0.0-20200121010907-a23fd04f2f68
)

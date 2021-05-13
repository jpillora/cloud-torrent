module cloud-torrent

go 1.16

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/anacrolix/log v0.9.0
	github.com/anacrolix/torrent v1.27.0
	github.com/boypt/scraper v1.0.3
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/fsnotify/fsnotify v1.4.9
	github.com/jpillora/archive v0.0.0-20160301031048-e0b3681851f1
	github.com/jpillora/cloud-torrent v0.0.0-20180427161701-1a741e3d8dd2
	github.com/jpillora/cookieauth v1.0.0
	github.com/jpillora/opts v1.2.0
	github.com/jpillora/requestlog v1.0.0
	github.com/jpillora/velox v0.4.0
	github.com/mmcdole/gofeed v1.1.3
	github.com/shirou/gopsutil v3.21.4+incompatible
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/viper v1.7.1
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
)

replace (
	github.com/boltdb/bolt => github.com/boypt/bolt v1.3.2
	github.com/jpillora/cloud-torrent => ./
	github.com/jpillora/cloud-torrent/engine => ./engine/
	github.com/jpillora/cloud-torrent/server => ./server/
	github.com/jpillora/cloud-torrent/static => ./static/
	github.com/jpillora/velox => github.com/boypt/velox v0.0.0-20200121010907-a23fd04f2f68
)

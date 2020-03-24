module cloud-torrent

go 1.13

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/anacrolix/log v0.6.0
	github.com/anacrolix/torrent v1.15.0
	github.com/boypt/scraper v1.0.3
	github.com/c2h5oh/datasize v0.0.0-20200112174442-28bbd4740fee
	github.com/elazarl/go-bindata-assetfs v1.0.0
	github.com/jpillora/archive v0.0.0-20160301031048-e0b3681851f1
	github.com/jpillora/cloud-torrent v0.0.0-20180427161701-1a741e3d8dd2
	github.com/jpillora/cookieauth v1.0.0
	github.com/jpillora/opts v1.1.2
	github.com/jpillora/requestlog v1.0.0
	github.com/jpillora/velox v0.3.3
	github.com/mmcdole/gofeed v1.0.0-beta2
	github.com/radovskyb/watcher v1.0.7
	github.com/shirou/gopsutil v2.20.2+incompatible
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/viper v1.6.2
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
)

replace (
	github.com/boltdb/bolt => github.com/boypt/bolt v1.3.2
	github.com/jpillora/cloud-torrent => ./
	github.com/jpillora/cloud-torrent/engine => ./engine/
	github.com/jpillora/cloud-torrent/server => ./server/
	github.com/jpillora/cloud-torrent/static => ./static/
	github.com/jpillora/velox => github.com/boypt/velox v0.0.0-20200121010907-a23fd04f2f68
)

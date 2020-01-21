module cloud-torrent

go 1.13

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/anacrolix/log v0.5.0
	github.com/anacrolix/torrent v1.11.0
	github.com/boypt/scraper v1.0.2
	github.com/c2h5oh/datasize v0.0.0-20171227191756-4eba002a5eae
	github.com/elazarl/go-bindata-assetfs v1.0.0
	github.com/jpillora/archive v0.0.0-20160301031048-e0b3681851f1
	github.com/jpillora/cloud-torrent v0.0.0-00010101000000-000000000000
	github.com/jpillora/cookieauth v0.0.0-20190219222732-2ae29b2a9c76
	github.com/jpillora/opts v1.1.0
	github.com/jpillora/requestlog v0.0.0-20181015073026-df8817be5f82
	github.com/jpillora/velox v0.0.0-20180825063758-42845d323220
	github.com/mmcdole/gofeed v1.0.0-beta2
	github.com/radovskyb/watcher v1.0.6
	github.com/shirou/gopsutil v2.18.12+incompatible
	github.com/skratchdot/open-golang v0.0.0-20190402232053-79abb63cd66e
	github.com/spf13/viper v1.6.1
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

module github.com/boypt/simple-torrent

go 1.16

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/StackExchange/wmi v0.0.0-20210224194228-fe8f1750fd46 // indirect
	github.com/ajg/form v1.5.1 // indirect
	github.com/anacrolix/log v0.9.0
	github.com/anacrolix/torrent v1.28.0
	github.com/andrew-d/go-termutil v0.0.0-20150726205930-009166a695a2 // indirect
	github.com/boypt/scraper v1.0.3
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/elithrar/simple-scrypt v1.3.0 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/gavv/httpexpect v2.0.0+incompatible // indirect
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/imkira/go-interpol v1.1.0 // indirect
	github.com/jpillora/ansi v1.0.2 // indirect
	github.com/jpillora/archive v0.0.0-20160301031048-e0b3681851f1
	github.com/jpillora/cookieauth v1.0.0
	github.com/jpillora/opts v1.2.0
	github.com/jpillora/requestlog v1.0.0
	github.com/jpillora/sizestr v1.0.0 // indirect
	github.com/jpillora/velox v0.4.1
	github.com/mmcdole/gofeed v1.1.3
	github.com/moul/http2curl v1.0.0 // indirect
	github.com/shirou/gopsutil/v3 v3.21.6
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/viper v1.7.1
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce // indirect
	github.com/valyala/fasthttp v1.28.0 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/yalp/jsonpath v0.0.0-20180802001716-5cc68e5049a0 // indirect
	github.com/yudai/gojsondiff v1.0.0 // indirect
	github.com/yudai/golcs v0.0.0-20170316035057-ecda9a501e82 // indirect
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace (
	github.com/anacrolix/dht/v2 => github.com/boypt/dht/v2 v2.9.2
	github.com/boltdb/bolt => github.com/boypt/bolt v1.3.2
	github.com/jpillora/velox => github.com/boypt/velox v0.0.0-20210702064309-321adb7223d6
	github.com/willf/bitset => github.com/bits-and-blooms/bitset v1.2.0
)

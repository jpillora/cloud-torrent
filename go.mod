module cloud-torrent

go 1.16

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/PuerkitoBio/goquery v1.6.1 // indirect
	github.com/StackExchange/wmi v0.0.0-20210224194228-fe8f1750fd46 // indirect
	github.com/ajg/form v1.5.1 // indirect
	github.com/anacrolix/log v0.9.0
	github.com/anacrolix/torrent v1.26.1
	github.com/andrew-d/go-termutil v0.0.0-20150726205930-009166a695a2 // indirect
	github.com/andybalholm/cascadia v1.2.0 // indirect
	github.com/boypt/scraper v1.0.3
	github.com/c2h5oh/datasize v0.0.0-20200825124411-48ed595a09d2
	github.com/elithrar/simple-scrypt v1.3.0 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/gavv/httpexpect v2.0.0+incompatible // indirect
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/imkira/go-interpol v1.1.0 // indirect
	github.com/jpillora/ansi v1.0.2 // indirect
	github.com/jpillora/archive v0.0.0-20160301031048-e0b3681851f1
	github.com/jpillora/cloud-torrent v0.0.0-20180427161701-1a741e3d8dd2
	github.com/jpillora/cookieauth v1.0.0
	github.com/jpillora/eventsource v1.1.0 // indirect
	github.com/jpillora/opts v1.2.0
	github.com/jpillora/requestlog v1.0.0
	github.com/jpillora/sizestr v1.0.0 // indirect
	github.com/jpillora/velox v0.4.0
	github.com/json-iterator/go v1.1.11 // indirect
	github.com/magiconair/properties v1.8.5 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/mmcdole/gofeed v1.1.3
	github.com/mmcdole/goxpp v0.0.0-20200921145534-2f3784f67354 // indirect
	github.com/moul/http2curl v1.0.0 // indirect
	github.com/pelletier/go-toml v1.9.0 // indirect
	github.com/pion/webrtc/v3 v3.0.28 // indirect
	github.com/posener/complete v1.2.3 // indirect
	github.com/radovskyb/watcher v1.0.7
	github.com/shirou/gopsutil v3.21.4+incompatible
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/afero v1.6.0 // indirect
	github.com/spf13/cast v1.3.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.7.1
	github.com/tidwall/gjson v1.7.5 // indirect
	github.com/tklauser/go-sysconf v0.3.5 // indirect
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce // indirect
	github.com/valyala/fasthttp v1.23.0 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/yalp/jsonpath v0.0.0-20180802001716-5cc68e5049a0 // indirect
	github.com/yudai/gojsondiff v1.0.0 // indirect
	github.com/yudai/golcs v0.0.0-20170316035057-ecda9a501e82 // indirect
	golang.org/x/crypto v0.0.0-20210505212654-3497b51f5e64 // indirect
	golang.org/x/net v0.0.0-20210505214959-0714010a04ed // indirect
	golang.org/x/sys v0.0.0-20210503173754-0981d6026fa6 // indirect
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	gomodules.xyz/jsonpatch/v2 v2.1.0 // indirect
	gopkg.in/ini.v1 v1.62.0 // indirect
)

replace (
	github.com/boltdb/bolt => github.com/boypt/bolt v1.3.2
	github.com/jpillora/cloud-torrent => ./
	github.com/jpillora/cloud-torrent/engine => ./engine/
	github.com/jpillora/cloud-torrent/server => ./server/
	github.com/jpillora/cloud-torrent/static => ./static/
	github.com/jpillora/velox => github.com/boypt/velox v0.0.0-20200121010907-a23fd04f2f68
)

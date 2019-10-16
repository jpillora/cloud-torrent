module cloud-torrent

go 1.12

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/ajg/form v1.5.1 // indirect
	github.com/anacrolix/log v0.3.1-0.20191001111012-13cede988bcd
	github.com/anacrolix/torrent v1.8.1
	github.com/boypt/scraper v0.0.0-20190916075425-61876a2e592e
	github.com/c2h5oh/datasize v0.0.0-20171227191756-4eba002a5eae
	github.com/elazarl/go-bindata-assetfs v1.0.0
	github.com/fasthttp-contrib/websocket v0.0.0-20160511215533-1f3b11f56072 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/gavv/httpexpect v2.0.0+incompatible // indirect
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/imkira/go-interpol v1.1.0 // indirect
	github.com/jpillora/archive v0.0.0-20160301031048-e0b3681851f1
	github.com/jpillora/cloud-torrent v0.0.0-00010101000000-000000000000
	github.com/jpillora/cookieauth v0.0.0-20190219222732-2ae29b2a9c76
	github.com/jpillora/opts v1.1.0
	github.com/jpillora/requestlog v0.0.0-20181015073026-df8817be5f82
	github.com/jpillora/velox v0.0.0-20180825063758-42845d323220
	github.com/k0kubun/colorstring v0.0.0-20150214042306-9440f1994b88 // indirect
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/moul/http2curl v1.0.0 // indirect
	github.com/onsi/ginkgo v1.10.2 // indirect
	github.com/onsi/gomega v1.7.0 // indirect
	github.com/radovskyb/watcher v1.0.6
	github.com/sergi/go-diff v1.0.0 // indirect
	github.com/shirou/gopsutil v2.18.12+incompatible
	github.com/skratchdot/open-golang v0.0.0-20190402232053-79abb63cd66e
	github.com/valyala/fasthttp v1.5.0 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/yalp/jsonpath v0.0.0-20180802001716-5cc68e5049a0 // indirect
	github.com/yudai/gojsondiff v1.0.0 // indirect
	github.com/yudai/golcs v0.0.0-20170316035057-ecda9a501e82 // indirect
	github.com/yudai/pp v2.0.1+incompatible // indirect
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
)

replace (
	github.com/boltdb/bolt => github.com/boypt/bolt v1.3.2
	github.com/jpillora/cloud-torrent => ./
	github.com/jpillora/cloud-torrent/engine => ./engine/
	github.com/jpillora/cloud-torrent/server => ./server/
	github.com/jpillora/cloud-torrent/static => ./static/
)

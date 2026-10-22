module github.com/jpillora/cloud-torrent

go 1.27.3

require (
	github.com/NYTimes/gziphandler v1.1.1
	github.com/anacrolix/torrent v1.61.0
	github.com/jpillora/archive v0.0.0-20160301031048-e0b3681851f1
	github.com/jpillora/backoff v1.0.0
	github.com/jpillora/cookieauth v1.1.1
	github.com/jpillora/opts v1.9.0
	github.com/jpillora/requestlog v1.0.0
	github.com/jpillora/scraper v0.3.0
	github.com/jpillora/velox v0.12.1
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
)

require (
	github.com/PuerkitoBio/goquery v1.13.0 // indirect
	github.com/RoaringBitmap/roaring v1.9.4 // indirect
	github.com/alecthomas/atomic v0.1.0-alpha2 // indirect
	github.com/anacrolix/btree v0.1.1 // indirect
	github.com/anacrolix/chansync v0.8.0 // indirect
	github.com/anacrolix/dht/v2 v2.24.0 // indirect
	github.com/anacrolix/envpprof v1.6.0 // indirect
	github.com/anacrolix/generics v0.2.0 // indirect
	github.com/anacrolix/go-libutp v1.5.1 // indirect
	github.com/anacrolix/log v0.17.1-0.20251118025802-918f1157b7bb // indirect
	github.com/anacrolix/missinggo v1.3.0 // indirect
	github.com/anacrolix/missinggo/perf v1.0.0 // indirect
	// v2.11 switches bitmap types, which torrent v1.61.0 does not support.
	github.com/anacrolix/missinggo/v2 v2.10.0 // indirect
	github.com/anacrolix/mmsg v1.1.1 // indirect
	github.com/anacrolix/multiless v0.4.0 // indirect
	// v0.6 removes Settish.Iter, which dht/v2 v2.24.0 still uses.
	github.com/anacrolix/stm v0.5.0 // indirect
	github.com/anacrolix/sync v0.6.0 // indirect
	github.com/anacrolix/upnp v0.1.4 // indirect
	github.com/anacrolix/utp v0.2.0 // indirect
	github.com/andrew-d/go-termutil v0.0.0-20150726205930-009166a695a2 // indirect
	github.com/andybalholm/cascadia v1.3.5 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/benbjohnson/immutable v0.4.3 // indirect
	github.com/bits-and-blooms/bitset v1.26.0 // indirect
	github.com/bradfitz/iter v0.0.0-20191230175014-e8f45d346db8 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/edsrzf/mmap-go v1.2.0 // indirect
	github.com/elithrar/simple-scrypt v1.4.1 // indirect
	github.com/felixge/fgprof v0.9.5 // indirect
	github.com/floatdrop/lru v1.3.0 // indirect
	github.com/go-llsqlite/adapter v0.2.0 // indirect
	github.com/go-llsqlite/crawshaw v0.7.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20261016150608-1facf8f91b50 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/huandu/xstrings v1.6.3 // indirect
	github.com/jpillora/ansi v1.0.3 // indirect
	github.com/jpillora/eventsource v1.2.0 // indirect
	github.com/jpillora/sizestr v1.0.0 // indirect
	github.com/klauspost/cpuid/v2 v2.4.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20260802145828-341c2f0c90b5 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mr-tron/base58 v1.3.0 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-varint v0.1.0 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/pion/datachannel v1.6.3 // indirect
	github.com/pion/dtls/v3 v3.1.15 // indirect
	github.com/pion/ice/v4 v4.4.8 // indirect
	github.com/pion/interceptor v0.1.49 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/mdns/v2 v2.2.2 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.20 // indirect
	github.com/pion/rtp v1.10.6 // indirect
	github.com/pion/sctp v1.12.0 // indirect
	github.com/pion/sdp/v3 v3.0.20 // indirect
	github.com/pion/srtp/v3 v3.1.3 // indirect
	github.com/pion/stun/v4 v4.0.1 // indirect
	github.com/pion/transport/v5 v5.1.1 // indirect
	github.com/pion/turn/v5 v5.1.2 // indirect
	github.com/pion/webrtc/v4 v4.2.25 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/posener/complete v1.2.3 // indirect
	github.com/power-devops/perfstat v0.0.0-20260916203055-22a1a467d9f0 // indirect
	github.com/protolambda/ctxlock v0.2.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rs/dnscache v0.0.0-20230804202142-fc85eb664529 // indirect
	github.com/shoenig/go-m1cpu v0.2.4 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/tidwall/btree v1.8.2 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/tomasen/realip v0.0.0-20180522021738-f0c99a92ddce // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/bbolt v1.5.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/exp v0.0.0-20261014143641-f0251eb4aa3e // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	lukechampine.com/blake3 v1.4.1 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.61.0 // indirect
	zombiezen.com/go/sqlite v1.4.2 // indirect
)

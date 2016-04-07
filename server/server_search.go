package server

//see github.com/jpillora/scraper for config specification
//cloud-torrent uses "<id>-item" handlers
var defaultSearchConfig = []byte(`{
	"kat": {
		"name": "Kickass Torrents",
		"url": "https://kat.cr/usearch/{{query}}/{{page:1}}/?field=seeders&sorder=desc",
		"list": "#mainSearchTable table tr[id]",
		"result": {
			"name":".cellMainLink",
			"path":[".cellMainLink", "@href"],
			"magnet": ["a[title=Torrent\\ magnet\\ link]", "@href"],
			"size": "td.nobr.center",
			"seeds": ".green.center",
			"peers": ".red.center"
		}
	},
	"tpb": {
		"name": "The Pirate Bay",
		"url": "https://thepiratebay.se/search/{{query}}/{{page:0}}/7//",
		"list": "#searchResult > tbody > tr",
		"result": {
			"name":"a.detLink",
			"path":["a.detLink","@href"],
			"magnet": ["a[title=Download\\ this\\ torrent\\ using\\ magnet]","@href"],
			"size": "/Size (\\d+(\\.\\d+).[KMG]iB)/",
			"seeds": "td:nth-child(3)",
			"peers": "td:nth-child(4)"
		}
	},
	"abb": {
		"name": "The Audiobook Bay",
		"url": "http://audiobookbay.co/page/{{page:1}}?s={{query}}",
		"list": "#content > div",
		"result": {
			"name":["div.postTitle > h2 > a","@title"],
			"path":["div.postTitle > h2 > a","@href"],
			"seeds": "div.postContent > p:nth-child(3) > span:nth-child(1)",
			"peers": "div.postContent > p:nth-child(3) > span:nth-child(3)"
		}
	},
	"abb-item": {
		"name": "The Audiobook Bay (Item)",
		"url": "http://audiobookbay.co{{path}}",
		"result": {
			"infohash": "/td>([a-f0-9]+)</",
			"tracker": "table tr td:nth-child(2)"
		}
	}
}`)

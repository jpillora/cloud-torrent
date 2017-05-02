# scraper

A configuration based, HTML ⇒ JSON API server

### Features

* Single binary
* Simple configuration
* Zero-downtime config reload with `kill -s SIGHUP <scraper-pid>`

### Install

**Binaries**

See [the latest release](https://github.com/jpillora/scraper/releases/latest) or download it with this one-liner: `curl i.jpillora.com/scraper | bash`

**Source**

``` sh
$ go get -v github.com/jpillora/scraper
```

### Quick Example

Given `google.json`

``` json
{
  "/search": {
    "url": "https://www.google.com/search?q={{query}}",
    "list": "#res div[class=g]",
    "result": {
      "title": "h3 a",
      "url": ["h3 a", "@href", "query-param(q)"]
    }
  }
}
```

``` sh
$ scraper google.json
2015/05/16 20:10:46 listening on 3000...
```

``` sh
$ curl "localhost:3000/search?query=hellokitty"
[
  {
    "title": "Official Home of Hello Kitty \u0026 Friends | Hello Kitty Shop",
    "url": "http://www.sanrio.com/"
  },
  {
    "title": "Hello Kitty - Wikipedia, the free encyclopedia",
    "url": "http://en.wikipedia.org/wiki/Hello_Kitty"
  },
  ...
```

### Configuration

``` plain
{
  <path>: {
    "url": <url>
    "list": <selector>,
    "result": {
      <field>: <extractor>,
      <field>: [<extractor>, <extractor>, ...],
      ...
    }
  }
}
```

* `<path>` - **Required** The path of the scraper
  * Accessible at `http://<host>:port/<path>`
  * You may define path variables like: `my/path/:var` when set to `/my/path/foo` then `:var = "foo"`
* `<url>` - **Required** The URL of the remote server to scrape
  * It may contain template variables in the form `{{ var }}`, scraper will look for a `var` path variable, if not found, it will then look for a query parameter `var`
* `result` - **Required** represents the resulting JSON object, after executing the `<extractor>` on the current DOM context. A field may use sequence of `<extractor>`s to perform more complex queries.
* `<extractor>` - A string in which must be one of:
  * a regex in form `/abc/` - searches the text of the current DOM context (extracts the first group when provided).
  * a regex in form `s/abc/xyz/` - searches the text of the current DOM context and replaces with the provided text (sed-like syntax).
  * an attribute in the form `@abc` - gets the attribute `abc` from the DOM context.
  * a query param in the form `query-param(abc)` - parses the current context as a URL and extracts the provided param
  * a css selector `abc` (if not in the forms above) alters the DOM context.
* `list` - **Optional** A css selector used to split the root DOM context into a set of DOM contexts. Useful for capturing search results.

#### Similar projects

*  https://github.com/ernesto-jimenez/scraperboard

#### MIT License

Copyright © 2016 &lt;dev@jpillora.com&gt;

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
'Software'), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

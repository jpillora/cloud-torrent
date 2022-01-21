/* globals app,window */
app.controller("OmniController", function (
  $scope,
  $rootScope,
  storage,
  api,
  apiget,
  search,
  reqinfo,
  rss
) {
  $rootScope.omni = $scope;
  $scope.inputs = {
    omni: storage.tcOmni || "",
    provider: storage.tcProvider || "tpb"
  };
  //edit fields
  $scope.edit = false;
  $scope.magnet = {
    trackers: [{ v: "" }]
  };
  $scope.providers = {};
  $scope.$watch("inputs.provider", function (p) {
    if (p) storage.tcProvider = p;
    $scope.parse();
  });
  apiget.searchproviders().then(function (xhr) {
    angular.forEach(xhr.data, function (val, k) {
      if (/\/item$/.test(k)) return;
      $scope.providers[k] = val;
    });
    $scope.SearchProvidersConfig = xhr.data;
    $scope.parse();
  });

  var parseTorrent = function () {
    $scope.mode.torrent = true;
  };

  var parseMagnet = function (params) {
    $scope.mode.magnet = true;
    var m = window.queryString.parse(params);

    if (!/^urn:btih:([A-Za-z0-9]+)$/.test(m.xt)) {
      $scope.omnierr = "Invalid Info Hash";
      return;
    }

    $scope.magnet.infohash = RegExp.$1;
    $scope.magnet.name = m.dn || "";
    //no trackers :O
    if (!m.tr) m.tr = [];
    //force array
    if (!(m.tr instanceof Array)) m.tr = [m.tr];

    //in place map
    for (var i = 0; i < m.tr.length; i++)
      $scope.magnet.trackers[i] = { v: m.tr[i] };

    while ($scope.magnet.trackers.length > m.tr.length)
      $scope.magnet.trackers.pop();

    $scope.magnet.trackers.push({ v: "" });
  };

  var parseSearch = function () {
    $scope.mode.search = true;
    $scope.results.length = 0;
  };

  $scope.clearSearch = function () {
    $scope.omnierr = null;
    $rootScope.err = null;
    $scope.inputs.omni = "";
    $scope.results.length = 0;
    $scope.mode.rss = false;
  };

  $scope.parse = function () {
    storage.tcOmni = $scope.inputs.omni;
    $scope.omnierr = null;
    $rootScope.err = null;
    var r = document.querySelector("#omni_search_results div.results");
    if (r !== null) {
      r.scrollTop = 0;
    }

    //set all 3 to false,
    //one will set to be true
    $scope.mode = {
      torrent: false,
      magnet: false,
      search: false,
      rss: false
    };
    $scope.page = 1;
    $scope.hasMore = true;
    $scope.noResults = false;
    $scope.results = [];

    if (/^https?:\/\//.test($scope.inputs.omni)) parseTorrent();
    else if (/^magnet:\?(.+)$/.test($scope.inputs.omni)) parseMagnet(RegExp.$1);
    else if ($scope.inputs.omni) parseSearch();
    else $scope.edit = false;
  };
  $scope.parse();

  var magnetURI = function (name, infohash, trackers) {
    return (
      "magnet:?" +
      "xt=urn:btih:" +
      (infohash || "") +
      "&" +
      "dn=" +
      encodeURIComponent(name) || "" +
      (trackers || [])
        .filter(function (t) {
          return !!t.v;
        })
        .map(function (t) {
          return "&tr=" + encodeURIComponent(t.v);
        })
        .join("")
    );
  };

  //get urls from the comma separated list
  var parseTrackers = function (result) {
    var trackers = (result.tracker || "")
      .split(",")
      .filter(function (s) {
        return /^(http|udp):\/\//.test(s);
      })
      .map(function (v) {
        return { v: v };
      });
    return trackers
  }

  $scope.parseMagnetString = function () {
    $scope.omnierr = null;
    if (!/^[A-Za-z0-9]+$/.test($scope.magnet.infohash)) {
      $scope.omnierr = "Invalid Info Hash";
      return;
    }
    for (var i = 0; i < $scope.magnet.trackers.length;)
      if (!$scope.magnet.trackers[i].v) $scope.magnet.trackers.splice(i, 1);
      else i++;
    $scope.inputs.omni = magnetURI(
      $scope.magnet.name,
      $scope.magnet.infohash,
      $scope.magnet.trackers
    );
    $scope.magnet.trackers.push({ v: "" });
    $scope.parse();
  };

  $scope.submitOmni = function () {
    if ($scope.mode.search) {
      $scope.submitSearch();
    } else {
      $scope.submitTorrent();
    }
  };

  $scope.submitTorrent = function () {
    if ($scope.mode.torrent) {
      api.url($scope.inputs.omni).then(reqinfo);
    } else if ($scope.mode.magnet) {
      api.magnet($scope.inputs.omni).then(reqinfo);
    } else {
      window.alert("UI Bug");
    }
    $rootScope.set_torrent_expanded(true);
  };

  $scope.submitSearch = function () {
    //lookup provider's origin
    var provider = $scope.SearchProvidersConfig[$scope.inputs.provider];
    if (!provider) return;
    var origin = /(https?:\/\/[^\/]+)/.test(provider.url) && RegExp.$1;
    var page = $scope.page;
    if (/{{page:0}}/.test(provider.url)) {
      page -= 1;
    }

    search
      .all($scope.inputs.provider, $scope.inputs.omni, page)
      .then(function (xhr) {
        var results = xhr.data;
        if (!results || results.length === 0) {
          $scope.noResults = true;
          $scope.hasMore = false;
          return;
        }
        for (var i = 0; i < results.length; i++) {
          var r = results[i];
          //add origin to path to create urls
          if (r.url && /^\//.test(r.url)) {
            if (!r.path) {
              r.path = r.url;
            }
            r.url = origin + r.url;
          } else if (r.path && /^\//.test(r.path)) {
            r.url = origin + r.path;
          }
          if (r.torrent && /^\//.test(r.torrent)) {
            r.torrent = origin + r.torrent;
          }
          $scope.results.push(r);
        }
        $scope.page++;
      });
  };

  $scope.submitTorrentItem = function (result) {
    if (result.torrent) {
      api.url(result.torrent).then(reqinfo);
    }
  }

  $scope.submitSearchItem = function (result) {
    //if search item has magnet/torrent, download now!
    if (result.magnet) {
      api.magnet(result.magnet);
      return;
    } else if (result.infohash) {
      api.magnet(magnetURI(result.name, result.infohash, parseTrackers(result))).then(reqinfo);
      return;
    } else if (result.torrent) {
      api.url(result.torrent).then(reqinfo);
      return;
    }
    //else, look it up via url path
    if (!result.path) return ($scope.omnierr = "No item URL found");

    search.one($scope.inputs.provider, result.path).then(
      function (resp) {
        var data = resp.data;
        if (!data) return ($scope.omnierr = "No response");
        if (data.torrent) return api.url(data.torrent);
        var magnet;
        if (data.magnet) {
          magnet = data.magnet;
        } else if (data.infohash) {
          magnet = magnetURI(result.name, data.infohash, parseTrackers(result));
        } else {
          $scope.omnierr = "No magnet or infohash found";
          return;
        }
        api.magnet(magnet).then(reqinfo);;
      },
      function (err) {
        $scope.omnierr = err;
      }
    );
    $rootScope.set_torrent_expanded(true);
  };

  $scope.get_rss = function (update) {
    if ($rootScope.searching) {
      return
    }
    storage.tcRSSGUID = $rootScope.state.LatestRSSGuid || "";
    $scope.omnierr = $rootScope.err = null;
    $scope.results.length = 0;
    if ($scope.mode.rss && !update) {
      $scope.mode.rss = false;
      return
    }
    $scope.parse();
    $scope.mode.search = true;
    $scope.mode.rss = true;
    rss.getrss(update).then(function (xhr) {
      results = xhr.data;
      $scope.hasMore = false;
      if (!results || results.length === 0) {
        $scope.noResults = true;
        return;
      }
      for (var i = 0; i < results.length; i++) {
        var r = results[i];
        r.seeds = $rootScope.ago(r.published);
        $scope.results.push(r);
      }
    });
  };

  // $var uploadFile = function(files) {

  // };

  // $scope.uploadEvent = function(element) {
  //     var files = [].slice.call(event.dataTransfer.files);
  //     uploadFiles(files);
  // };
  // var files = [].slice.call(event.dataTransfer.files);
  // if (files.length === 0 || !files[0].name.endsWith(".torrent")) {
  //   alert("file must be a .torrent file");
  //   return;
  // }
  // var reader = new FileReader();
  // reader.onload = function() {
  //   var data = new Uint8Array(reader.result);
  //   element.value = null;
  //   api.torrentfile(data);
  // };
});

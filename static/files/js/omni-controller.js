/* globals app,window */

app.controller("OmniController", function($scope, $rootScope, storage, api, search) {
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
  $scope.$watch("inputs.provider", function(p) {
    if(p) storage.tcProvider = p;
    $scope.parse();
  });
  //if unset, set to first provider
  $rootScope.$watch("state.SearchProviders", function(searchProviders) {
    //remove last set
    if(!searchProviders)
      return;
    //filter
    for(var id in searchProviders) {
      if(/-item$/.test(id)) continue;
      $scope.providers[id] = searchProviders[id];
    }
    $scope.parse();
  });

  var parseTorrent = function() {
    $scope.mode.torrent = true;
  };

  var parseMagnet = function(params) {
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

  var parseSearch = function() {
    $scope.mode.search = true;
    while ($scope.results.length)
      $scope.results.pop();
  };

  $scope.parse = function() {
    storage.tcOmni = $scope.inputs.omni;
    $scope.omnierr = null;
    //set all 3 to false,
    //one will set to be true
    $scope.mode = {
      torrent: false,
      magnet: false,
      search: false
    };
    $scope.page = 1;
    $scope.hasMore = true;
    $scope.noResults = false;
    $scope.results = [];

    if (/^https?:\/\//.test($scope.inputs.omni))
      parseTorrent();
    else if (/^magnet:\?(.+)$/.test($scope.inputs.omni))
      parseMagnet(RegExp.$1);
    else if ($scope.inputs.omni)
      parseSearch();
    else
      $scope.edit = false;
  };
  $scope.parse();

  var magnetURI = function(name, infohash, trackers) {
    return "magnet:?" +
      "xt=urn:btih:" + (infohash || '') + "&" +
      "dn=" + (name || '').replace(/\W/g, '').replace(/\s+/g, '+') +
      (trackers || []).filter(function(t) {
        return !!t.v;
      }).map(function(t) {
        return "&tr=" + encodeURIComponent(t.v);
      }).join('');
  };

  $scope.parseMagnetString = function() {
    $scope.omnierr = null;
    if (!/^[A-Za-z0-9]+$/.test($scope.magnet.infohash)) {
      $scope.omnierr = "Invalid Info Hash";
      return;
    }
    for (var i = 0; i < $scope.magnet.trackers.length;)
      if (!$scope.magnet.trackers[i].v)
        $scope.magnet.trackers.splice(i, 1);
      else
        i++;
    $scope.inputs.omni = magnetURI($scope.magnet.name, $scope.magnet.infohash, $scope.magnet.trackers);
    $scope.magnet.trackers.push({ v: "" });
  };

  $scope.submitOmni = function() {
    if($scope.mode.search) {
      $scope.submitSearch();
    } else {
      $scope.submitTorrent();
    }
  };

  $scope.submitTorrent = function() {
    if($scope.mode.torrent) {
      api.url($scope.inputs.omni);
    } else if($scope.mode.magnet) {
      api.magnet($scope.inputs.omni);
    } else {
      window.alert("UI Bug");
    }
  };

  $scope.submitSearch = function() {

    //lookup provider's origin
    var provider = $scope.state.SearchProviders[$scope.inputs.provider];
    if(!provider) return;
    var origin = /(https?:\/\/[^\/]+)/.test(provider.url) && RegExp.$1;

    search.all($scope.inputs.provider, $scope.inputs.omni, $scope.page).success(function(results) {
      if (!results || results.length === 0) {
        $scope.noResults = true;
        $scope.hasMore = false;
        return;
      }
      for (var i = 0; i < results.length; i++) {
        var r = results[i];
        //add origin to path to create urls
        if(r.path && /^\//.test(r.path)) {
          r.url = origin + r.path;
        }
        $scope.results.push(r);
      }
      $scope.page++;
    });

  };

  $scope.submitSearchItem = function(result) {
    //if search item has magnet, download now!
    if (result.magnet) {
      api.magnet(result.magnet);
      return;
    }
    //else, look it up via url
    if (!result.path)
      return $scope.omnierr = "No URL found";

    search.one($scope.inputs.provider, result.path).then(function(resp) {
      var data = resp.data;
      if (!data)
        return $scope.omnierr = "No response";
      var magnet;
      if (data.magnet) {
        magnet = data.magnet;
      } else if (data.infohash) {
        //get urls from the comma separated list
        var trackers = (data.tracker || "").split(",").filter(function(s){
          return /^(http|udp):\/\//.test(s);
        }).map(function(v) { return {v:v}; });
        magnet = magnetURI(result.name, data.infohash, trackers);
      } else {
        $scope.omnierr = "No magnet or infohash found";
        return;
      }
      api.magnet(magnet);
    }, function(err) {
      $scope.omnierr = err;
    });
  };
});

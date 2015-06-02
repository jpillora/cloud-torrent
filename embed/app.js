var window = this;

var app = window.angular.module('app', []);

app.filter('keys', function() {
	return Object.keys;
});

app.filter('filename', function() {
	return function(path) {
		return (/\/([^\/]+)$/).test(path) ? RegExp.$1 : path;
	};
});

app.filter('bytes', function(bytes) {
	return bytes;
});

app.factory('bytes', function() {
	var scale = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
	return function(n) {
		var i = 0;
		var s = scale[i];
		if (typeof n !== 'number') {
			return "-";
		}
		while (n > 1000) {
			s = scale[++i] || 'x10^' + (i * 3);
			n = Math.round(n / 100) / 10;
		}
		return "" + n + " " + s;
	};
});

app.directive('ngEnter', function() {
	return function(scope, element, attrs) {
		element.bind("keydown keypress", function(event) {
			if (event.which === 13) {
				scope.$apply(function() {
					scope.$eval(attrs.ngEnter);
				});
				event.preventDefault();
			}
		});
	};
});

//TODO remove this hack
app.directive('jpSrc', function() {
	return function(scope, element, attrs) {
		scope.$watch(attrs.jpSrc, function(src) {
			element.attr("src", src);
		});
	};
});

app.factory('storage', function() {
	return window.localStorage || {};
});

app.factory('reqerr', function() {
	return function(err) {
		console.error("req-error", err);
	};
});

app.factory('api', function($rootScope, $http, reqerr) {
	var request = function(action, data) {
		var url = "/api/"+$rootScope.engineID+"/"+action;
		$rootScope.apiing = true;
		return $http.post(url, data).error(reqerr).finally(function() {
			$rootScope.apiing = false;
		});
	};
	var api = {};
	["magnet","url","list","fetch"].forEach(function(action) {
		api[action] = request.bind(null, action);
	});
	return api;
});

app.factory('search', function($rootScope, $http, reqerr) {
	return function(suffix, params) {
		var opts = { params: params };
		var url = "/search/"+$rootScope.omni.provider+(suffix||"");
		$rootScope.searching = true;
		return $http.get(url, opts).error(reqerr).finally(function() {
			$rootScope.searching = false;
		});
	};
});

app.controller("OmniController", function($scope, $rootScope, storage) {
	$rootScope.omni = $scope;
	$scope.omni = storage.tcOmni || "";
	//edit fields
	$scope.edit = false;
	$scope.trackers = [{ v: "" }];
	$scope.provider = storage.tcProvider || "";
	$scope.$watch("provider", function(p) {
		if(p)	storage.tcProvider = p;
		$scope.parse();
	});
	//if unset, set to first provider
	$rootScope.$watch("data.providers", function(providers) {
		if ($scope.provider) return;
		for (var id in providers) break;
		$scope.provider = id;
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

		$scope.infohash = RegExp.$1;
		$scope.name = m.dn || "";
		//no trackers :O
		if (!m.tr)
			return;
		//force array
		if (!(m.tr instanceof Array))
			m.tr = [m.tr];

		//in place map
		for (var i = 0; i < m.tr.length; i++)
			$scope.trackers[i] = { v: m.tr[i] };

		while ($scope.trackers.length > m.tr.length)
			$scope.trackers.pop();

		$scope.trackers.push({ v: "" });
	};

	var parseSearch = function() {
		$scope.mode.search = true;
		while ($scope.results.length)
			$scope.results.pop();
	};

	$scope.parse = function() {
		storage.tcOmni = $scope.omni;
		$scope.omnierr = null;
		$scope.mode = {
			torrent: false,
			magnet: false,
			search: false
		};
		$scope.page = 1;
		$scope.hasMore = true;
		$scope.noResults = false;
		$scope.results = [];

		if (/^https?:\/\//.test($scope.omni))
			parseTorrent();
		else if (/^magnet:\?(.+)$/.test($scope.omni))
			parseMagnet(RegExp.$1);
		else if ($scope.omni)
			parseSearch();
		else
			$scope.edit = false;
	};
	$scope.parse();

	var magnetURI = function(name, infohash, trackers) {
		return "magnet:?" +
			"xt=urn:btih:" + (infohash || '') + "&" +
			"dn=" + (name || '').replace(/\W/g, '').replace(/\s+/g, '+') +
			(trackers || []).map(function(t) {
				return "&tr=" + encodeURIComponent(t.v);
			}).join('');
	};

	$scope.stringify = function() {
		$scope.omnierr = null;

		if (!/^[A-Za-z0-9]+$/.test($scope.infohash)) {
			$scope.omnierr = "Invalid Info Hash";
			return;
		}

		for (var i = 0; i < $scope.trackers.length;)
			if (!$scope.trackers[i].v)
				$scope.trackers.splice(i, 1);
			else
				i++;

		$scope.omni = magnetURI($scope.name, $scope.infohash, $scope.trackers);
		$scope.trackers.push({ v: "" });
	};

	$scope.searchList = function() {
		$scope.searchAPI("list", {
			provider: $scope.provider,
			query: $scope.omni,
			page: $scope.page
		}, function(err, results) {
			if (err)
				return $scope.omnierr = err;
			if (results.length === 0) {
				$scope.noResults = true;
				$scope.hasMore = false;
				return;
			}
			for (var i = 0; i < results.length; i++) {
				$scope.results.push(results[i]);
			}
			$scope.page++;
		});
	};

	$scope.searchLoad = function(result) {
		//if search item has magnet, download now!
		if (result.magnet) {
			$scope.torrentsAPI("load", {
				magnet: result.magnet
			});
			return;
		}
		//else, look it up via url
		if (!result.url)
			return $scope.omnierr = "No URL found";

		$scope.searchAPI("item", {
			provider: $scope.provider,
			url: result.url
		}, function(err, data) {
			if (err)
				return $scope.omnierr = err;

			var load = {};

			if (data.magnet) {
				load.magnet = data.magnet;
			} else if (data.infohash) {
				load.magnet = magnetURI(result.name, data.infohash, [{
					v: data.tracker
				}]);
			} else {
				$scope.omnierr = "No magnet or infohash found";
				return;
			}

			$scope.torrentsAPI("load", load);
		});
	};
});

//RootController
app.run(function($rootScope, search, api) {

	var $scope = window.scope = $rootScope;
	$scope.data = {};
	$scope.engineID = "native";
	$scope.search = search;
	$scope.api = api;

	$scope.uploaded = function(f) {
		var path = typeof f === "object" ? f.path : f;
		return $scope.data.uploads && $scope.data.uploads[path];
	};

	$scope.sumFiles = function(t) {
		return t.files ? t.files.reduce(function(s, f) { return s+f.length; }, 0) : 0;
	};

	$scope.previews = {};
	$scope.ext = function(path) {
		return (/\.([^\.]+)$/).test(path) ? RegExp.$1 : null;
	};

	var rt = window.realtime.sync($scope.data);

	rt.onstatus = function(s) {
		$scope.connected = s;
		$scope.$apply();
	};

	//handle disconnects/re-tries

});
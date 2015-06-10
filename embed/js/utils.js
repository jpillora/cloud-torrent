/* globals app,window */

app.factory('api', function($rootScope, $http, reqerr) {
  var request = function(action, data) {
    var url = "/api/"+$rootScope.engineID+"/"+action;
    $rootScope.apiing = true;
    return $http.post(url, data).error(reqerr).finally(function() {
      $rootScope.apiing = false;
    });
  };
  var api = {};
  var actions = ["configure","magnet","url","list","fetch"];
  actions.forEach(function(action) {
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

app.factory('storage', function() {
  return window.localStorage || {};
});

app.factory('reqerr', function() {
  return function(err) {
    console.error("req-error", err);
  };
});

app.filter('keys', function() {
  return Object.keys;
});

app.filter('addspaces', function() {
  return function(s) {
    if(typeof s !== "string")
      return s;
    return s.replace(/([A-Z]+[a-z]*)/g, function(_, word) {
      return " " + word;
    }).replace(/^\ /, "");
  };
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

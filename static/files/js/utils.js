/* globals app,window */

app.factory("api", function($rootScope, $http, reqerr) {
  window.http = $http;
  var request = function(action, data) {
    var url = "api/" + action;
    $rootScope.apiing = true;
    return $http
      .post(url, data)
      .error(reqerr)
      .finally(function() {
        $rootScope.apiing = false;
      });
  };
  var api = {};
  var actions = ["configure", "magnet", "url", "torrent", "file"];
  actions.forEach(function(action) {
    api[action] = request.bind(null, action);
  });
  return api;
});

app.factory("search", function($rootScope, $http, reqerr) {
  return {
    all: function(provider, query, page) {
      var params = {query: query};
      if (page !== undefined) params.page = page;
      $rootScope.searching = true;
      var req = $http.get("search/" + provider, {params: params});
      req.error(reqerr);
      req.finally(function() {
        $rootScope.searching = false;
      });
      return req;
    },
    one: function(provider, path) {
      var opts = {params: {item: path}};
      $rootScope.searching = true;
      var req = $http.get("search/" + provider + "/item", opts);
      req.error(reqerr);
      req.finally(function() {
        $rootScope.searching = false;
      });
      return req;
    }
  };
});

app.factory("storage", function() {
  return window.localStorage || {};
});

app.factory("reqerr", function() {
  return function(err, status) {
    alert(err.error || err);
    console.error("request error '%s' (%s)", err, status);
  };
});

app.filter("keys", function() {
  return Object.keys;
});

app.filter("addspaces", function() {
  return function(s) {
    if (typeof s !== "string") return s;
    return s
      .replace(/([A-Z]+[a-z]*)/g, function(_, word) {
        return " " + word;
      })
      .replace(/^\ /, "");
  };
});

app.filter("filename", function() {
  return function(path) {
    return /\/([^\/]+)$/.test(path) ? RegExp.$1 : path;
  };
});

app.filter("bytes", function(bytes) {
  return bytes;
});

//scale a number  and add a metric prefix
app.factory("bytes", function() {
  return function(n, d) {
    // set defaults
    if (typeof n !== "number" || isNaN(n) || n == 0) return "0 B";
    if (!d || typeof d !== "number") d = 1;
    // set scale index 1000,100000,... becomes 1,2,...
    var i = Math.floor(Math.floor(Math.log(n) * Math.LOG10E) / 3);
    // set rounding factor
    var f = Math.pow(10, d);
    // scale n down and round
    var s = Math.round(n / Math.pow(10, i * 3) * f) / f;
    // concat (no trailing 0s) and choose scale letter
    return (
      s.toString().replace(/\.0+$/, "") +
      " " +
      ["", "K", "M", "G", "T", "P", "Z"][i] +
      "B"
    );
  };
});

app.filter("round", function() {
  return function(n) {
    if (typeof n !== "number") {
      return n;
    }
    return Math.round(n * 10) / 10;
  };
});

app.directive("ngEnter", function() {
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
app.directive("jpSrc", function() {
  return function(scope, element, attrs) {
    scope.$watch(attrs.jpSrc, function(src) {
      element.attr("src", src);
    });
  };
});

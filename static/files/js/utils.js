/* globals app,window */

app.factory("api", function ($rootScope, $http, reqerr) {
  window.http = $http;
  var request = function (action, data) {
    var url = "api/" + action;
    $rootScope.apiing = true;
    var req = $http.post(url, data, {
      transformRequest: []
    })
    req.error(reqerr);
    req.finally(function () {
      $rootScope.apiing = false;
    });
    return req;
  };
  var api = {};
  var actions = [
    "configure",
    "magnet",
    "url",
    "torrent",
    "file",
    "torrentfile"
  ];
  actions.forEach(function (action) {
    api[action] = request.bind(null, action);
  });
  return api;
});


app.factory("apiget", function ($rootScope, $http, reqerr) {
  var request = function (action, data) {
    var url = "api/" + action;
    $rootScope.apiing = true;
    var req = $http.get(url);
    req.error(reqerr);
    req.finally(function () {
      $rootScope.apiing = false;
    });
    return req;
  };
  var api = {};
  var actions = [
    "enginedebug"
  ];
  actions.forEach(function (action) {
    api[action] = request.bind(null, action);
  });
  return api;
});

app.factory("search", function ($rootScope, $http, reqerr) {
  return {
    all: function (provider, query, page) {
      var params = { query: query };
      if (page !== undefined) params.page = page;
      $rootScope.searching = true;
      var req = $http.get("search/" + provider, { params: params });
      req.error(reqerr);
      req.finally(function () {
        $rootScope.searching = false;
      });
      return req;
    },
    one: function (provider, path) {
      var opts = { params: { item: path } };
      $rootScope.searching = true;
      var req = $http.get("search/" + provider + "/item", opts);
      req.error(reqerr);
      req.finally(function () {
        $rootScope.searching = false;
      });
      return req;
    }
  };
});

app.factory("rss", function ($rootScope, $http, reqerr) {
  return {
    getrss: function (update) {
      $rootScope.searching = true;
      var config = { "params": { _: Date.now() }, cache: false };
      if (update) {
        config["params"]["update"] = 1;
      }
      var req = $http.get("rss", config);
      req.error(reqerr);
      req.finally(function () {
        $rootScope.searching = false;
      });
      return req;
    }
  };
});

app.factory("storage", function () {
  return window.localStorage || {};
});

app.factory("reqerr", function ($rootScope) {
  return function (err, status) {
    var msg = err;
    if (typeof err === "object" && "error" in err) {
      msg = err.error;
    }
    $rootScope.err = `${msg} - (${status})`
    $rootScope.$apply();
    console.log(msg, status);
  };
});

app.filter("keys", function () {
  return Object.keys;
});

app.filter("addspaces", function () {
  return function (s) {
    return typeof s !== "string"
      ? s
      : s
        .replace(/([A-Z]+[a-z]*)/g, function (_, word) {
          return " " + word;
        })
        .replace(/^\ /, "");
  };
});

app.filter("filename", function () {
  return function (path) {
    return /\/([^\/]+)$/.test(path) ? RegExp.$1 : path;
  };
});

app.filter("bytes", function (bytes) {
  return bytes;
});

//scale a number  and add a metric prefix
app.factory("bytes", function () {
  return function (bytes) {
    if (bytes == 0) return '0 B';
    var k = 1024,
      dm = 0,
      sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'],
      i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
  };
});

app.filter("round", function () {
  return function (n) {
    if (typeof n !== "number") {
      return n;
    }
    return Math.round(n * 10) / 10;
  };
});

app.filter('dictValuesArray', function () {
  return function (obj) {
    if (!(obj instanceof Object)) return obj;

    var arr = [];
    for (var key in obj) {
      arr.push(obj[key]);
    }
    return arr;
  }
})

app.directive("ngEnter", function () {
  return function (scope, element, attrs) {
    element.bind("keydown keypress", function (event) {
      if (event.which === 13) {
        scope.$apply(function () {
          scope.$eval(attrs.ngEnter);
        });
        event.preventDefault();
      }
    });
  };
});

//TODO remove this hack
app.directive("jpSrc", function () {
  return function (scope, element, attrs) {
    scope.$watch(attrs.jpSrc, function (src) {
      element.attr("src", src);
    });
  };
});

app.directive("ondropfile", function () {
  return {
    restrict: "A",
    link: function (scope, elem, attrs) {
      if (!window.FileReader) {
        return console.info("File API not available");
      }
      var placeholder = attrs.placeholder || "Drop your file here";
      //prepare cover
      var cover = angular.element("<div>");
      cover.addClass("file-drop-cover");
      var dots = angular.element("<div>");
      dots.addClass("dots");
      cover.append(dots);
      var msg = angular.element("<div>");
      msg.addClass("msg");
      msg.text(placeholder);
      dots.append(msg);
      elem.prepend(cover);
      //bind to events
      elem.on("dragenter", function (e) {
        cover.addClass("shown");
        e.preventDefault();
        e.dataTransfer.dropEffect = "copy";
      });
      function remove() {
        cover.removeClass("shown");
      }
      elem.on("drop", function (event) {
        event.preventDefault();
        scope.$eval(attrs.ondropfile, {
          $event: event
        });
        remove();
      });
      elem.on("dragover", function (e) {
        //move "drop here"
        var y = e.pageY - elem[0].offsetTop - 60;
        msg.css({ top: y + "px" });
        //queue remove
        clearTimeout(remove.t);
        remove.t = setTimeout(remove, 300);
        e.preventDefault();
      });
    }
  };
});

app.directive("onfileclick", function () {
  return {
    restrict: "A",
    link: function (scope, elem, attrs) {
      if (!window.FileReader) {
        console.info("File API not available");
        return;
      }
      //create hidden file input
      var file = angular.element("<input type='file'>");
      if ("multiple" in attrs) {
        file.attr("multiple", attrs.multiple);
      }
      if ("accept" in attrs) {
        file.attr("accept", attrs.accept);
      }
      file.css("display", "none");
      file.on("change", function (event) {
        event.preventDefault();
        scope.$eval(attrs.onfileclick, {
          $event: event
        });
        file[0].value = null;
      });
      elem.append(file);
      //proxy click from elem to file
      elem.on("click", function () {
        file[0].click();
      });
    }
  };
});

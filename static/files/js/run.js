/* globals app,window */

//RootController
app.run(function ($rootScope, $window, $location, $log, search, api, apiget, storage, reqinfo, reqerr) {
  var $scope = (window.scope = $rootScope);

  // register as "magnet:" protocol handler
  // only available when visited as a https site
  if ('registerProtocolHandler' in $window.navigator) {
    var handlurl = $location.absUrl() + 'api/magnet?m=%s';
    $window.navigator.registerProtocolHandler('magnet', handlurl, 'SimpleTorrent');
    $log.info("Registered protocol handler", handlurl);
  }

  // velox event stream framework
  $scope.state = {};
  $scope.hasConnected = false;
  var v, vtype = storage.veloxCON || "sse";
  var syncLoc = $location.absUrl() + "sync";
  if (vtype == "ws") {
    v = velox.ws(syncLoc, $scope.state);
    $log.info("Velox is using Websocket")
  } else {
    v = velox.sse(syncLoc, $scope.state);
    $log.info("Velox is using EventStream")
  }
  v.onupdate = function () {
    $scope.$applyAsync();
  };
  v.onchange = function (connected) {
    if (connected) {
      $scope.hasConnected = true;
    }
    $scope.$applyAsync(function () {
      $scope.connected = connected;
    });
  };

  $scope.DownloadingFiles = {};
  $scope.$watch("state.Torrents", function (newobj, oldobj) {
    $scope.DownloadingFiles = {};
    if (!angular.isObject(newobj) || angular.equals(newobj, {})) {
      return
    }

    angular.forEach(newobj, function (tval) {
      angular.forEach(tval.Files, function (fval) {
        if (fval.Percent < 100) {
          var base = fval.Path.split(/[\\/]/).pop()
          $scope.DownloadingFiles[base] = true;
        }
      });
    });
  }, true)
  //expose services
  $scope.search = search;
  $scope.api = api;
  $scope.storage = storage;

  $scope.ready = function (f) {
    var path = typeof f === "object" ? f.path : f;
    return $scope.state.Uploads && $scope.state.Uploads[path];
  };

  $scope.previews = {};
  $scope.ext = function (path) {
    return /\.([^\.]+)$/.test(path) ? RegExp.$1 : null;
  };

  $scope.isEmpty = function (obj) {
    return $scope.numKeys(obj) === 0;
  };

  $scope.numKeys = function (obj) {
    return obj ? Object.keys(obj).length : 0;
  };

  $scope.ago = function (t) {
    return moment(t).fromNow();
  };

  $scope.agoHrs = function (t) {
    return moment().diff(moment(t), "hours");
  };

  $scope.withHrs = function (t, hrs) {
    return $scope.agoHrs(t) <= hrs;
  };

  $scope.etaTime = function (dled, total, dlrate) {
    if (dlrate <= 0) {
      return "Infinite"
    }
    etaSec = Math.round((total - dled) / dlrate)
    return moment().add(etaSec, 'seconds').fromNow();
  };

  $scope.uploadTorrent = function (event) {
    var fileContainer = event.dataTransfer || event.target;
    if (!fileContainer || !fileContainer.files) {
      return $rootScope.alertErr("Invalid file event");
    }
    var filter = Array.prototype.filter;
    var files = filter.call(fileContainer.files, function (file) {
      return file.name.endsWith(".torrent");
    });
    if (files.length === 0) {
      return $rootScope.alertErr("No torrent files to upload");
    }
    files.forEach(function (file) {
      var reader = new FileReader();
      reader.readAsArrayBuffer(file);
      reader.onload = function () {
        var data = new Uint8Array(reader.result);
        api.torrentfile(data).then(reqinfo, reqerr);
      };
    });
  };

  $scope.alertErr = function (errMsg) {
    $scope.err = errMsg;
    $scope.$applyAsync();
    return false;
  }

  $scope.toggleSections = function (section) {
    $scope.err = null;
    $scope.info = null;
    switch (section) {
      case "rss":
        $rootScope.omni.get_rss(false)
        break;
      case "config":
        $rootScope.config.edit = !$rootScope.config.edit;
        if ($rootScope.config.edit) {
          apiget.configure().then(function (xhr) {
            $rootScope.config.configObj = xhr.data;
          })
        }
        break
      case "omni":
        $rootScope.omni.edit = !$rootScope.omni.edit;
        break
      case "enginedebug":
        $rootScope.showEnineStatus = !$rootScope.showEnineStatus;
        if ($rootScope.showEnineStatus) {
          $rootScope.EngineStatus = $scope.Trackers = "loading...";
          apiget.enginedebug().then(function (xhr) {
            $scope.EngineStatus = xhr.data.EngineStatus;
            $scope.Trackers = xhr.data.Trackers.join("\n");
          });
        }
        break
    }
  }

  $scope.toggleWebsocket = function () {
    storage.veloxCON = (storage.veloxCON !== "ws") ? "ws" : "sse";
    switch (storage.veloxCON) {
      case "ws":
        $rootScope.info = "Websocket mode, refresh to take affect.";
        break
      case "sse":
        $rootScope.info = "Eventstream mode, refresh to take affect.";
        break
    }
    $rootScope.$applyAsync();
  }

  //page-wide keybinding, listen for space,
  //toggle pause/play the video on-screen
  document.addEventListener("keydown", function (e) {
    if (e.code !== "Space") { // space key
      return;
    }
    var typing = document.querySelector("input:focus");
    if (typing) {
      return;
    }
    var height = window.innerHeight || document.documentElement.clientHeight;
    var medias = document.querySelectorAll("video,audio");
    for (var i = 0; i < medias.length; i++) {
      var media = medias[i];
      var rect = media.getBoundingClientRect();
      var inView =
        (rect.top >= 0 && rect.top <= height) ||
        (rect.bottom >= 0 && rect.bottom <= height);
      if (!inView) {
        continue;
      }
      if (media.paused) {
        media.play();
      } else {
        media.pause();
      }
      e.preventDefault();
      break;
    }
  });

});

app.config([
  '$compileProvider',
  function ($compileProvider) {
    $compileProvider.aHrefSanitizationWhitelist(/^\s*(https?|ftp|mailto|magnet):/);
  }
]);


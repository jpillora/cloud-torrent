/* globals app,window */

//RootController
app.run(function ($rootScope, search, api, apiget, storage) {
  var $scope = (window.scope = $rootScope);

  var pn = window.location.pathname
  if (pn[pn.length - 1] != "/") {
    pn += "/"
  }

  // register as "magnet:" protocol handler
  if ('registerProtocolHandler' in navigator) {
    navigator.registerProtocolHandler(
      'magnet',
      document.location.origin + pn + 'api/magnet?m=%s',
      'SimpleTorrent'
    );
  }

  // velox event stream framework
  $scope.state = {};
  $scope.hasConnected = false;
  var v = velox(pn + "sync", $scope.state);
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
  //expose services
  $scope.search = search;
  $scope.api = api;
  $scope.storage = storage;

  $scope.inputType = function (k, v) {
    multiLines = ["RssURL"];
    if (multiLines.includes(k)) {
      return "multiline"
    }

    switch (typeof v) {
      case "number":
        return "number";
      case "boolean":
        return "check";
    }
    return "text";
  };

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
        api.torrentfile(data);
      };
    });
  };

  $scope.alertErr = function (errMsg) {
    $scope.err = errMsg;
    $scope.$apply();
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
        break
      case "omni":
        $rootScope.omni.edit = !$rootScope.omni.edit;
        break
      case "enginedebug":
        $rootScope.showEnineStatus = !$rootScope.showEnineStatus;
        if ($rootScope.showEnineStatus) {
          $rootScope.EngineStatus = "loading...";
          apiget.enginedebug().success(function (data) {
            $rootScope.EngineStatus = data;
          })
        }
        break
    }
  }

  //page-wide keybinding, listen for space,
  //toggle pause/play the video on-screen
  document.addEventListener("keydown", function (e) {
    if (e.keyCode !== 32) {
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


/* globals app,window */

//RootController
app.run(function($rootScope, search, api) {
  var $scope = (window.scope = $rootScope);

  //velox
  $scope.state = {};
  $scope.hasConnected = false;
  var v = velox("/sync", $scope.state);
  v.onupdate = function() {
    $scope.$applyAsync();
  };
  v.onchange = function(connected) {
    if (connected) {
      $scope.hasConnected = true;
    }
    $scope.$applyAsync(function() {
      $scope.connected = connected;
    });
  };
  //expose services
  $scope.search = search;
  $scope.api = api;

  $scope.inputType = function(v) {
    switch (typeof v) {
      case "number":
        return "number";
      case "boolean":
        return "check";
    }
    return "text";
  };

  $scope.ready = function(f) {
    var path = typeof f === "object" ? f.path : f;
    return $scope.state.Uploads && $scope.state.Uploads[path];
  };

  $scope.previews = {};
  $scope.ext = function(path) {
    return /\.([^\.]+)$/.test(path) ? RegExp.$1 : null;
  };

  $scope.isEmpty = function(obj) {
    return $scope.numKeys(obj) === 0;
  };

  $scope.numKeys = function(obj) {
    return obj ? Object.keys(obj).length : 0;
  };

  $scope.ago = function(t) {
    return moment(t).fromNow();
  };

  $scope.agoHrs = function(t) {
    return moment().diff(moment(t), "hours");
  };

  $scope.withHrs = function(t, hrs) {
    return $scope.agoHrs(t) <= hrs;
  };

  $scope.uploadTorrent = function(event) {
    var fileContainer = event.dataTransfer || event.target;
    if (!fileContainer || !fileContainer.files) {
      return alert("Invalid file event");
    }
    var filter = Array.prototype.filter;
    var files = filter.call(fileContainer.files, function(file) {
      return file.name.endsWith(".torrent");
    });
    if (files.length === 0) {
      return alert("No torrent files to upload");
    }
    files.forEach(function(file) {
      var reader = new FileReader();
      reader.readAsArrayBuffer(file);
      reader.onload = function() {
        var data = new Uint8Array(reader.result);
        api.torrentfile(data);
      };
    });
  };

  //page-wide keybinding, listen for space,
  //toggle pause/play the video on-screen
  document.addEventListener("keydown", function(e) {
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

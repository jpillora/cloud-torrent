/* globals app */

app.controller("TorrentsController", function ($scope, $rootScope, api, reqinfo, reqerr) {
  $rootScope.torrents = $scope;

  $scope.submitTorrent = function (action, t) {
    api.torrent([action, t.InfoHash].join(":")).then(function (xhr) {
      console.log(`${action}:${xhr.data}`);
    }).finally(function () {
      t.showSubBtns = false;
    });
  };

  $scope.submitFile = function (action, t, f) {
    api.file([action, t.InfoHash, f.Path].join(":")).then(reqinfo, reqerr);
  };

  $scope.downloading = function (f) {
    return f.Completed > 0 && f.Completed < f.Size;
  };

  $scope.copyMagnetLink = function ($event) {
    $event.currentTarget.previousElementSibling.select();
    return document.execCommand('copy');
  };

  $scope.toggleTagDetail = function ($event, item) {
    var tg = $event.currentTarget;
    var tagTitle = tg.getAttribute("title");
    var showMode = tg.dataset["mode"];
    if (tagTitle === item.$detailTitle) {
      item.$showMode = "";
      item.$detailTitle = "";
    } else {
      item.$showMode = showMode;
      item.$detailTitle = tagTitle;
    }
    return false;
  };

  $scope.onDeleteBtnClick = function (t) {
    if (!$rootScope.state.UseQueue) {
      $scope.submitTorrent("delete", t);
      return
    }
    t.showSubBtns = !(t.showSubBtns);
  }

  $scope.$expanded = true;
  $scope.section_expanded_toggle = function () {
    $scope.$expanded = !$scope.$expanded;
  };
  $rootScope.set_torrent_expanded = function (isExpand) {
    $scope.$expanded = isExpand;
  }
});

/* globals app */

app.controller("TorrentsController", function($scope, $rootScope, api) {
  $rootScope.torrents = $scope;

  $scope.submitTorrent = function(action, t) {
    api.torrent([action, t.InfoHash].join(":"));
  };

  $scope.submitFile = function(action, t, f) {
    api.file([action, t.InfoHash, f.Path].join(":"));
  };

  $scope.downloading = function(f) {
    return f.Completed > 0 && f.Completed < f.Chunks;
  };

  $scope.copyMagnetLink = function($event) {
    $event.currentTarget.previousElementSibling.select();
    return document.execCommand('copy');
  };

  $scope.showMode = function($event, item) {
    var tg = $event.currentTarget;
    item.$showMode = tg.dataset["mode"];
    item.$detailTitle = tg.getAttribute("title");
    return false;
  };

  $scope.$expanded = true;
  $scope.section_expanded_toggle = function() {
    $scope.$expanded = !$scope.$expanded;
  };
  $rootScope.set_torrent_expanded = function(isExpand) {
    $scope.$expanded = isExpand;
  }
});

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

  // $scope.downloading = function(f) {
  //   return f.Completed > 0 && f.Completed < f.Chunks;
  // };
});

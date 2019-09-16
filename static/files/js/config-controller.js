/* globals app,window */

app.controller("ConfigController", function($scope, $rootScope, storage, api) {
  $rootScope.config = $scope;
  $scope.edit = false;
  $scope.configOrderdKey = [
    "AutoStart",
    "EnableSeeding",
    "EnableUpload",
    "IncomingPort",
    "ObfsPreferred",
    "ObfsRequirePreferred",
    "DisableTrackers",
    "DisableIPv6",
    "SeedRatio",
    "UploadRate",
    "DownloadRate",
    "DownloadDirectory",
    "WatchDirectory",
    "TrackerListURL",
    "AlwaysAddTrackers"
  ];

  $scope.toggle = function(b) {
    $scope.edit = b === undefined ? !$scope.edit : b;
  };
  $scope.submitConfig = function() {
    var data = JSON.stringify($rootScope.state.Config);
    api.configure(data);
  };
});

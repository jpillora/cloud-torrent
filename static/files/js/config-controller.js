/* globals app,window */

app.controller("ConfigController", function($scope, $rootScope, storage, api) {
  $rootScope.config = $scope;
  $scope.edit = false;
  $scope.configOrderdKey = [
    "AutoStart",
    "DisableEncryption",
    "EnableSeeding",
    "EnableUpload",
    "IncomingPort",
    "SeedRatio",
    "UploadRate",
    "DownloadRate",
    "DownloadDirectory",
    "WatchDirectory"
  ];

  $scope.toggle = function(b) {
    $scope.edit = b === undefined ? !$scope.edit : b;
  };
  $scope.submitConfig = function() {
    var data = JSON.stringify($rootScope.state.Config);
    api.configure(data);
  };
});

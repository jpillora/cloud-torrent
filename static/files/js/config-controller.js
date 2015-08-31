/* globals app,window */

app.controller("ConfigController", function($scope, $rootScope, storage, api) {
  $rootScope.config = $scope;
  $scope.edit = false;
  $scope.toggle = function(b) {
    $scope.edit = b === undefined ? !$scope.edit : b;
  };
  $scope.submitConfig = function() {
    api.configure($rootScope.state.Config);
  };
});

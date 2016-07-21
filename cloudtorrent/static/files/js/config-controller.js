/* globals app,window */

app.controller("ConfigController", function($scope, $rootScope, storage, api) {
  var hash = function(obj) {
    return JSON.stringify(obj, function(k,v) {
      return k.charAt(0) === "$" ? undefined : v;
    });
  };
  //inputs is a copy of configurations
  $scope.inputs = {};
  $scope.saved = true;
  $scope.$watch("inputs", function() {
    $scope.saved = hash($scope.cfgs) === hash($scope.inputs);
  }, true);
  //
  $rootScope.$watch("state.Configurations", function(cfgs) {
    $scope.cfgs = cfgs || {};
    $scope.cfgsHash = hash($scope.cfgs);
    $scope.inputs = angular.copy($scope.cfgs);
    $scope.saved = true;
  }, true);

  $scope.submitConfig = function() {
    api.configure($scope.inputs);
  };
});

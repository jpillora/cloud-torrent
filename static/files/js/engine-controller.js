/* globals app,window */

app.controller("EngineController", function($scope, $rootScope, storage, api) {
  $rootScope.engine = $scope;
  $scope.edit = false;
  $scope.id = "native";

  $scope.submitConfig = function() {

    if(!$rootScope.data.Engines)
      return;
    if(!$scope.id)
      return;

    var e = $rootScope.data.Engines[$scope.id];
    if(!e)
      return;

    var update = {};
    update[$scope.id] = JSON.stringify(e.Config, null, 2);

    api.configure(update);
  };
});

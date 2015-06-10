/* globals app,window */

app.controller("EngineController", function($scope, $rootScope, storage, api) {
  $rootScope.engine = $scope;
  $scope.edit = true;
  $scope.engineID = "native";

  $scope.submitConfig = function() {

    if(!$rootScope.data.Engines)
      return;
    if(!$scope.engineID)
      return;

    var e = $rootScope.data.Engines[$scope.engineID];
    if(!e)
      return;

    var update = {};
    update[$scope.engineID] = JSON.stringify(e.Config, null, 2);

    api.configure(update);
  };
});

/* globals app,window */

//RootController
app.run(function($rootScope, search, api) {

  var $scope = window.scope = $rootScope;

  //setup realtime data
  $scope.data = {};
  $scope.data.$onupdate = function() {
    //TODO throttle $applys
    $scope.$apply();
  };

  var rt = window.realtime.sync($scope.data);
  rt.onstatus = function(isConnected) {
    $scope.connected = isConnected;
    $scope.$apply();
  };

  //expose services
  $scope.search = search;
  $scope.api = api;

  $scope.inputType = function(v) {
    switch(typeof v) {
      case "number":
        return "number";
      case "boolean":
        return "check";
    }
    return "text";
  };

  $scope.ready = function(f) {
    var path = typeof f === "object" ? f.path : f;
    return $scope.data.uploads && $scope.data.uploads[path];
  };

  $scope.previews = {};
  $scope.ext = function(path) {
    return (/\.([^\.]+)$/).test(path) ? RegExp.$1 : null;
  };

  $scope.isEmpty = function(obj) {
    if(!obj)
      return true;
    return Object.keys(obj).length === 0;
  };
});
/* globals app */

app.controller("DownloadsController", function($scope, $rootScope) {
  $rootScope.downloads = $scope;

  $scope.numDownloads = function() {
    if($scope.state.Downloads && $scope.state.Downloads.Children)
      return $scope.state.Downloads.Children.length;
    return 0;
  };
});

app.controller("NodeController", function($scope, $rootScope, $http) {
  var n = $scope.node;
  var path = [n.Name];
  if($scope.$parent && $scope.$parent.$parent && $scope.$parent.$parent.node) {
    var parentNode = $scope.$parent.$parent.node;
    path.unshift(parentNode.$path);
    n.$depth = parentNode.$depth + 1;
  } else {
    n.$depth = 1;
  }
  n.$path = path.join("/");
  n.$closed = $scope.agoHrs(n.Modified) > 24;

  //defaults
  $scope.closed = function() { return n.$closed; };
  $scope.toggle = function() { n.$closed = !n.$closed; };
  $scope.icon = function() {
    var c = ["outline","icon"];
    if($scope.isfile()) {
      c.push("file");
    } else {
      c.push("folder");
    }
    if(!$scope.closed()) {
      c.push("open");
    }
    return c.join(" ");
  };
  $scope.isfile = function() { return !n.Children; };
  $scope.isdir = function() { return !$scope.isfile(); };

  $scope.remove = function() {
    $http.delete("/download/" + n.$path);
  };
});

/* globals app,window */

//RootController
app.run(function($rootScope, search, api) {

  var $scope = window.scope = $rootScope;

  //link up to angular
	var rt = realtime("/realtime");
	$scope.state = {};
	rt.add("state", $scope.state, function() {
		$scope.$apply();
	});
	rt.onstatus = function(online) {
		$scope.$apply(function() {
			$scope.connected = online;
		});
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
    return $scope.state.Uploads && $scope.state.Uploads[path];
  };

  $scope.previews = {};
  $scope.ext = function(path) {
    return (/\.([^\.]+)$/).test(path) ? RegExp.$1 : null;
  };

  $scope.isEmpty = function(obj) {
    return $scope.numKeys(obj) === 0;
  };

  $scope.numKeys = function(obj) {
    return obj ? Object.keys(obj).length : 0;
  };

	$scope.ago = function(t) {
		return moment(t).fromNow();
	};

  $scope.agoHrs = function(t) {
    return moment().diff(moment(t), 'hours');
  };

  $scope.withHrs = function(t, hrs) {
    return $scope.agoHrs(t) <= hrs;
  };
});

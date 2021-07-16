/* globals app,window */

app.controller("ConfigController", function ($scope, $rootScope, api) {
  $rootScope.config = $scope;
  $scope.configObj = {};
  $scope.edit = false;
  $scope.configOrderdKey = [
    "AutoStart",
    "EnableSeeding",
    "EnableUpload",
    "DisableTrackers",
    "MaxConcurrentTask",
    "SeedRatio",
    "UploadRate",
    "DownloadRate",
    "TrackerList",
    "AlwaysAddTrackers",
    "RssURL"
  ];

  $scope.configAttr = {
    "AutoStart": { t: "check", desc: "Whether to start task when added." },
    "EnableSeeding": { t: "check", desc: "Upload even after there's nothing in it for us." },
    "EnableUpload": { t: "check", desc: "Upload data we have." },
    "DisableTrackers": { t: "check", desc: "Don't announce to trackers. This only leaves DHT to discover peers." },
    "MaxConcurrentTask": { t: "number", desc: "Maxmium downloading torrent tasks allowed." },
    "SeedRatio": { t: "number", desc: "The ratio of task Upload/Download data when reached, the task will be stopped." },
    "UploadRate": { t: "text", desc: "Upload speed limiter, Low(~50k/s), Medium(~500k/s) and High(~1500k/s) is accepted , Unlimited / 0 or empty result in unlimited rate, or a customed value eg: 850k/720kb/2.85MB. " },
    "DownloadRate": { t: "text", desc: "Download speed limiter, Low(~50k/s), Medium(~500k/s) and High(~1500k/s) is accepted , Unlimited / 0 or empty result in unlimited rate, or a customed value eg: 850k/720kb/2.85MB. " },
    "TrackerList": { t: "multiline", desc: "A list of trackers to add to torrents, prefix with \"remote:\" will be retrived with http." },
    "AlwaysAddTrackers": { t: "check", desc: "Whether add trackers even there are trackers specified in the torrent/magnet" },
    "RssURL": { t: "multiline", desc: "A newline seperated list of magnet RSS feeds. (http/https)" }
  };

  $scope.toggle = function (b) {
    $scope.edit = b === undefined ? !$scope.edit : b;
  };
  $scope.submitConfig = function () {
    var data = JSON.stringify($scope.configObj);
    api.configure(data).then(function (xhr) {
      $rootScope.info = `${xhr.data}: Config Saved`;
    }).finally(function () {
      $scope.edit = false;
    });
  };
});

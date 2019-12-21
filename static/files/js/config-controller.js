/* globals app,window */

app.controller("ConfigController", function ($scope, $rootScope, storage, api) {
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
    "ProxyURL",
    "TrackerListURL",
    "AlwaysAddTrackers",
    "RssURL"
  ];

  $scope.configTip = {
    "AutoStart": "Whether to start task when added.",
    "EnableSeeding": "Upload even after there's nothing in it for us.",
    "EnableUpload": "Upload data we have.",
    "IncomingPort": "The incomming port peers connects to.",
    "ObfsPreferred": "Whether header obfuscation is preferred",
    "ObfsRequirePreferred": "Whether the value of ObfsPreferred is a strict requirement",
    "DisableTrackers": "Don't announce to trackers. This only leaves DHT to discover peers.",
    "DisableIPv6": "Dont't linten and connect with IPv6",
    "SeedRatio": "The ratio of task Upload/Download data when reached, the task will be stopped.",
    "UploadRate": "Upload speed limiter, Low(~50k/s), Medium(~500k/s) and High(~1500k/s) is accepted , Unlimited / 0 or empty result in unlimited rate, or a customed value eg: 850k/720kb/2.85MB. ",
    "DownloadRate": "Download speed limiter, Low(~50k/s), Medium(~500k/s) and High(~1500k/s) is accepted , Unlimited / 0 or empty result in unlimited rate, or a customed value eg: 850k/720kb/2.85MB. ",
    "DownloadDirectory": "The directory where downloaded file saves.",
    "WatchDirectory": "The directory SimpleTorrent will watch and load the new added .torrent files",
    "ProxyURL": "Proxy URL",
    "TrackerListURL": "A https URL to a trackers list, this option is design to retrive public trackers from github.com/ngosang/trackerslist. ",
    "AlwaysAddTrackers": "Whether add trackers even there are trackers specified in the torrent/magnet",
    "RssURL": "A newline seperated list of magnet RSS feeds. (http/https)"
  };

  $scope.toggle = function (b) {
    $scope.edit = b === undefined ? !$scope.edit : b;
  };
  $scope.submitConfig = function () {
    $rootScope.info = null;
    $rootScope.err = null;
    var data = JSON.stringify($rootScope.state.Config);
    api.configure(data).success(function (data, status, headers, config) {
      $scope.edit = false;
      $rootScope.info = `${data}: Config Saved`;
    });
  };
});

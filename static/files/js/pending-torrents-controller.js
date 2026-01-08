/* globals app */

app.controller("PendingTorrentsController", function($scope, $rootScope, api) {
    $rootScope.torrents = $scope;
  
    $scope.submitTorrent = function(action, t) {
      api.torrent([action, t.InfoHash].join(":"));
    };
  
    $scope.submitFile = function(action, t, f) {
      api.file([action, t.InfoHash, f.Path].join(":"));
    };

    $scope.startTorrentWithFiles = function(t) {
      api.torrentWithFiles(["start", t.InfoHash, t.Files.filter(function(f) {
        return f.selected;
      }).map(function(f) {
        return f.FilePosition;
      })].join(":"));
    };
    $scope.deleteTorrent = function(t) {
      api.torrentWithFiles(["delete", t.InfoHash].join(":"));
    };
  
    // Check if any files are selected in a torrent
    $scope.hasSelectedFiles = function(t) {
      if (!t.Files) return false;
      return t.Files.some(function(f) {
        return f.selected;
      });
    };
  
    // Submit selected files for download
    $scope.submitSelectedFiles = function(t) {
      if (!t.Files) return;
      t.Files.forEach(function(f) {
        if (f.selected) {
          $scope.submitFile('start', t, f);
        }
      });
    };
    $scope.numberFilesSelected = function(t) {
      if (!t.Files) return 0;
      return t.Files.filter(function(f) {
        return f.selected;
      }).length;
    };
    $scope.totalSizeSelected = function(t) {
      if (!t.Files) return 0;
      return t.Files.filter(function(f) {
        return f.selected;
      }).reduce(function(sum, f) {
        return sum + f.Size;
      }, 0);
    };
  
    // Toggle select all files in a torrent
    $scope.toggleSelectAll = function(t) {
      if (!t.Files) return;
      
      t.Files.forEach(function(f) {
        if (f.Percent < 100) { // Only select files that aren't already downloaded
          f.selected = t.$selectAll;
        }
      });
    };
  });
  
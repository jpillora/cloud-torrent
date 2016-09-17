

<!-- MAGNET EDITOR -->
<div class="ui segment" ng-show="edit" >
  <form class="ui edit form">
    <h4 class="ui dividing header">Magnet URI Editor</h4>
    <div class="field">
      <label>Name</label>
      <div class="field">
        <input type="text"
        ng-model="magnet.name"
        ng-change="parseMagnetString()"
        placeholder="Name">
      </div>
    </div>
    <div class="field">
      <label>Info Hash</label>
      <div class="field">
        <input type="text"
        ng-model="magnet.infohash"
        ng-change="parseMagnetString()"
        placeholder="Info Hash">
      </div>
    </div>
    <div class="field">
      <label>Trackers</label>
    </div>
    <div>
      <div class="field" ng-repeat="t in magnet.trackers">
        <input type="text"
        ng-model="t.v"
        ng-change="parseMagnetString()"
        placeholder="Tracker">
      </div>
    </div>
  </form>
</div>

<!-- OMNI BAR -->
<div class="omni ui fluid icon input">
  <input
    placeholder="Enter search query, torrent URL or magnet URI"
    ng-model="inputs.omni"
    ng-change="parse()"
    ng-enter="submitOmni()"/>
  <i class="icon"
    ng-class="{search: mode.search, magnet: mode.magnet || mode.torrent,
          help: !mode.search && !mode.magnet && !mode.torrent}"></i>
</div>

<!-- MAGNET FIELD ERROR -->
<div ng-show="omnierr" class="ui error message">
  <p>{{omnierr}}</p>
</div>

<!-- START TORRENT BUTTONS -->
<div class="search buttons" ng-show="mode.torrent">
  <div ng-click="submitTorrent()"
    class="ui tiny blue button"
    ng-class="{loading: apiing, disabled: apiing }">
    <span ng-show="mode.torrent">Start Torrent</span>
  </div>
</div>

<!-- SEARCH BUTTONS -->
<div class="search buttons" ng-show="mode.search && inputs.provider">
  <select class="ui green button" ng-model="inputs.provider"
    ng-options="id as s.name for (id, s) in providers">
  </select>
  <div ng-click="submitSearch()"
    class="ui tiny blue button"
    ng-class="{loading: searching,
      disabled: noResults || hasMore && results && results.length > 0
    }">
    <span ng-show="noResults">No results!</span>
    <span ng-show="!noResults">Search</span>
  </div>
</div>

<div class="ui error message" ng-show="mode.search && !inputs.provider">
  <p>You have no search providers</p>
</div>

<!-- SEARCH RESULTS -->
<div class="results" ng-show="mode.search && results && results.length > 0" >
  <table class="ui unstackable compact striped tcld table">
    <tr ng-repeat="r in results">
      <td class="name">
        <a ng-href="{{ r.url }}" target="_blank">{{ r.name }}</a>
      </td>
      <td ng-if="r.size" class="size">
        {{ r.size }}
      </td>
      <td class="users">
        <span class="seeds">{{ r.seeds }}</span><br/>
        <span class="peers"> {{ r.peers }}</span>
      <td class="controls">
        <i ng-click="submitSearchItem(r)"
          class="ui green download icon"></i>
      </td>
    </tr>
    <tr ng-if="hasMore">
      <td class="loadmore" colspan="10">
        <div ng-click="submitSearch()"
          class="ui tiny blue button"
          ng-class="{loading: searching}">
          Load more
        </div>
      </td>
    </tr>
  </table>
</div>

<!-- MAGNET BUTTONS -->
<div class="magnet buttons" ng-show="mode.magnet">
  <div ng-click="submitTorrent()"
    class="ui tiny blue button"
    ng-class="{loading: apiing}">
    Load Magnet
  </div>
  <div ng-show="mode.magnet"
    ng-click="edit = !edit"
    ng-class="{green: edit}"
    class="ui tiny button">Edit</div>
</div>

<!-- TORRENT BUTTON -->
<div class="buttons" ng-show="torrent">
  <div ng-click="submitTorrent()"
    class="ui tiny blue button"
    ng-class="{loading: apiing}">
    Load Torrent
  </div>
</div>

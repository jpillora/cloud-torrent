const { createApp } = Vue;

createApp({
  data() {
    return {
      state: {},
      hasConnected: false,
      connected: false,
      err: null,
      apiing: false,
      searching: false,
      logoHover: false,
      previews: {},
      config: { edit: false },
      omni: { 
        edit: false,
        inputs: {
          omni: window.localStorage.getItem('tcOmni') || "",
          provider: window.localStorage.getItem('tcProvider') || "tpb"
        },
        magnet: { trackers: [{ v: "" }] },
        providers: {},
        mode: { torrent: false, magnet: false, search: false },
        page: 1,
        hasMore: true,
        noResults: false,
        results: [],
        omnierr: null
      },
      torrents: {},
      downloads: {}
    };
  },
  mounted() {
    this.initVelox();
    this.initKeyboardListener();
    this.parseOmniInput();
  },
  watch: {
    'state.SearchProviders': {
      handler(searchProviders) {
        if (!searchProviders) return;
        // filter providers
        for (let id in searchProviders) {
          if (/\/item$/.test(id)) continue;
          this.omni.providers[id] = searchProviders[id];
        }
        this.parseOmniInput();
      },
      deep: true
    }
  },
  methods: {
    initVelox() {
      const v = velox("/sync", this.state);
      v.onupdate = () => {
        this.$nextTick();
      };
      v.onchange = (connected) => {
        if (connected) {
          this.hasConnected = true;
        }
        this.connected = connected;
      };
    },
    
    initKeyboardListener() {
      document.addEventListener("keydown", (e) => {
        if (e.keyCode !== 32) return;
        const typing = document.querySelector("input:focus");
        if (typing) return;
        
        const height = window.innerHeight || document.documentElement.clientHeight;
        const medias = document.querySelectorAll("video,audio");
        
        for (let i = 0; i < medias.length; i++) {
          const media = medias[i];
          const rect = media.getBoundingClientRect();
          const inView = (rect.top >= 0 && rect.top <= height) || 
                        (rect.bottom >= 0 && rect.bottom <= height);
          if (!inView) continue;
          
          if (media.paused) {
            media.play();
          } else {
            media.pause();
          }
          e.preventDefault();
          break;
        }
      });
    },

    inputType(v) {
      switch (typeof v) {
        case "number": return "number";
        case "boolean": return "check";
        default: return "text";
      }
    },

    ready(f) {
      const path = typeof f === "object" ? f.path : f;
      return this.state.Uploads && this.state.Uploads[path];
    },

    ext(path) {
      return /\.([^\.]+)$/.test(path) ? RegExp.$1 : null;
    },

    isEmpty(obj) {
      return this.numKeys(obj) === 0;
    },

    numKeys(obj) {
      return obj ? Object.keys(obj).length : 0;
    },

    ago(t) {
      return moment(t).fromNow();
    },

    agoHrs(t) {
      return moment().diff(moment(t), "hours");
    },

    withHrs(t, hrs) {
      return this.agoHrs(t) <= hrs;
    },

    formatBytes(n, d = 1) {
      if (typeof n !== "number" || isNaN(n) || n == 0) return "0 B";
      const i = Math.floor(Math.floor(Math.log(n) * Math.LOG10E) / 3);
      const f = Math.pow(10, d);
      const s = Math.round(n / Math.pow(10, i * 3) * f) / f;
      return s.toString().replace(/\.0+$/, "") + " " + ["", "K", "M", "G", "T", "P", "Z"][i] + "B";
    },

    uploadTorrent(event) {
      const fileContainer = event.dataTransfer || event.target;
      if (!fileContainer || !fileContainer.files) {
        return alert("Invalid file event");
      }
      
      const files = Array.from(fileContainer.files).filter(file => 
        file.name.endsWith(".torrent")
      );
      
      if (files.length === 0) {
        return alert("No torrent files to upload");
      }
      
      files.forEach(file => {
        const reader = new FileReader();
        reader.readAsArrayBuffer(file);
        reader.onload = () => {
          const data = new Uint8Array(reader.result);
          this.apiRequest("torrentfile", data);
        };
      });
    },

    // API methods - bind to component instance
    apiRequest(action, data) {
      const url = "api/" + action;
      this.apiing = true;
      return axios({
        method: "POST",
        url: url,
        data: data,
        transformRequest: []
      })
      .catch(err => {
        alert(err.response?.data?.error || err.message);
        console.error("request error", err);
      })
      .finally(() => {
        this.apiing = false;
      });
    },

    // Search methods
    searchAll(provider, query, page) {
      const params = { query: query };
      if (page !== undefined) params.page = page;
      this.searching = true;
      
      return axios.get("search/" + provider, { params })
        .catch(err => {
          alert(err.response?.data?.error || err.message);
          console.error("search error", err);
        })
        .finally(() => {
          this.searching = false;
        });
    },

    searchOne(provider, path) {
      const params = { item: path };
      this.searching = true;
      
      return axios.get("search/" + provider + "/item", { params })
        .catch(err => {
          alert(err.response?.data?.error || err.message);
          console.error("search error", err);
        })
        .finally(() => {
          this.searching = false;
        });
    },

    // Controller methods
    submitConfig() {
      const data = JSON.stringify(this.state.Config);
      this.apiRequest("configure", data);
    },

    submitTorrent(action, t) {
      this.apiRequest("torrent", [action, t.InfoHash].join(":"));
    },

    submitFile(action, t, f) {
      this.apiRequest("file", [action, t.InfoHash, f.Path].join(":"));
    },

    submitOmni() {
      if (this.omni.mode.search) {
        this.submitSearch();
      } else {
        this.submitTorrentFromOmni();
      }
    },

    submitTorrentFromOmni() {
      if (this.omni.mode.torrent) {
        this.apiRequest("url", this.omni.inputs.omni);
      } else if (this.omni.mode.magnet) {
        this.apiRequest("magnet", this.omni.inputs.omni);
      } else {
        alert("UI Bug");
      }
    },

    submitSearch() {
      const provider = this.state.SearchProviders?.[this.omni.inputs.provider];
      if (!provider) return;
      const origin = /(https?:\/\/[^\/]+)/.test(provider.url) && RegExp.$1;

      this.searchAll(this.omni.inputs.provider, this.omni.inputs.omni, this.omni.page)
        .then(response => {
          const results = response?.data;
          if (!results || results.length === 0) {
            this.omni.noResults = true;
            this.omni.hasMore = false;
            return;
          }
          
          results.forEach(r => {
            if (r.url && /^\//.test(r.url)) {
              r.path = r.url;
              r.url = origin + r.path;
            }
            if (r.torrent && /^\//.test(r.torrent)) {
              r.torrent = origin + r.torrent;
            }
            this.omni.results.push(r);
          });
          this.omni.page++;
        });
    },

    submitSearchItem(result) {
      if (result.magnet) {
        this.apiRequest("magnet", result.magnet);
        return;
      } else if (result.torrent) {
        this.apiRequest("url", result.torrent);
        return;
      }

      if (!result.path) {
        this.omni.omnierr = "No item URL found";
        return;
      }

      this.searchOne(this.omni.inputs.provider, result.path)
        .then(resp => {
          const data = resp.data;
          if (!data) {
            this.omni.omnierr = "No response";
            return;
          }
          if (data.torrent) {
            this.apiRequest("url", data.torrent);
            return;
          }
          
          let magnet;
          if (data.magnet) {
            magnet = data.magnet;
          } else if (data.infohash) {
            const trackers = (data.tracker || "")
              .split(",")
              .filter(s => /^(http|udp):\/\//.test(s))
              .map(v => ({ v }));
            magnet = this.magnetURI(result.name, data.infohash, trackers);
          } else {
            this.omni.omnierr = "No magnet or infohash found";
            return;
          }
          this.apiRequest("magnet", magnet);
        })
        .catch(err => {
          this.omni.omnierr = err.message;
        });
    },

    magnetURI(name, infohash, trackers) {
      return "magnet:?" +
        "xt=urn:btih:" + (infohash || "") +
        "&dn=" + (name || "").replace(/\W/g, "").replace(/\s+/g, "+") +
        (trackers || [])
          .filter(t => !!t.v)
          .map(t => "&tr=" + encodeURIComponent(t.v))
          .join("");
    },

    parseOmniInput() {
      // Parse the omni input on component load
      window.localStorage.setItem('tcOmni', this.omni.inputs.omni);
      this.omni.omnierr = null;
      this.omni.mode = { torrent: false, magnet: false, search: false };
      this.omni.page = 1;
      this.omni.hasMore = true;
      this.omni.noResults = false;
      this.omni.results = [];

      if (/^https?:\/\//.test(this.omni.inputs.omni)) {
        this.omni.mode.torrent = true;
      } else if (/^magnet:\?(.+)$/.test(this.omni.inputs.omni)) {
        this.parseMagnet(RegExp.$1);
      } else if (this.omni.inputs.omni) {
        this.omni.mode.search = true;
      } else {
        this.omni.edit = false;
      }
    },

    parseMagnet(params) {
      this.omni.mode.magnet = true;
      const searchParams = new URLSearchParams(params);

      const xt = searchParams.get('xt') || '';
      if (!/^urn:btih:([A-Za-z0-9]+)$/.test(xt)) {
        this.omni.omnierr = "Invalid Info Hash";
        return;
      }

      this.omni.magnet.infohash = RegExp.$1;
      this.omni.magnet.name = searchParams.get('dn') || "";
      
      const trackers = searchParams.getAll('tr') || [];
      this.omni.magnet.trackers = trackers.map(tr => ({ v: tr }));
      this.omni.magnet.trackers.push({ v: "" });
    }
  },

  components: {
    'config-section': {
      template: `
        <section class="config" v-if="config.edit">
          <form class="ui segment edit form">
            <h4 class="ui dividing header">Configuration</h4>
            <div v-for="(v, k) in state.Config" :key="k" class="field">
              <div v-if="inputType(v) === 'check'">
                <div class="ui toggle checkbox">
                  <input type="checkbox" v-model="state.Config[k]">
                  <label>{{ addSpaces(k) }}</label>
                </div>
              </div>
              <div v-else>
                <label>{{ addSpaces(k) }}</label>
                <input :type="inputType(v)" v-model="state.Config[k]">
              </div>
            </div>
            <div class="buttons">
              <div class="ui blue button" :class="{loading: apiing}" @click="$emit('submit-config')">
                Save
              </div>
              <div class="ui grey button" @click="config.edit = false">
                Cancel
              </div>
            </div>
          </form>
        </section>
      `,
      props: ['state', 'config', 'apiing'],
      methods: {
        inputType(v) {
          switch (typeof v) {
            case "number": return "number";
            case "boolean": return "check";
            default: return "text";
          }
        },
        addSpaces(s) {
          return typeof s !== "string" ? s : 
            s.replace(/([A-Z]+[a-z]*)/g, (_, word) => " " + word)
             .replace(/^\ /, "");
        }
      }
    },

    'omni-section': {
      template: `
        <section class="omni">
          <!-- MAGNET EDITOR -->
          <div class="ui segment" v-if="omni.edit">
            <form class="ui edit form">
              <h4 class="ui dividing header">Magnet URI Editor</h4>
              <div class="field">
                <label>Name</label>
                <div class="field">
                  <input type="text" v-model="omni.magnet.name" @input="parseMagnetString" placeholder="Name">
                </div>
              </div>
              <div class="field">
                <label>Info Hash</label>
                <div class="field">
                  <input type="text" v-model="omni.magnet.infohash" @input="parseMagnetString" placeholder="Info Hash">
                </div>
              </div>
              <div class="field">
                <label>Trackers</label>
              </div>
              <div>
                <div class="field" v-for="(t, index) in omni.magnet.trackers" :key="index">
                  <input type="text" v-model="t.v" @input="parseMagnetString" placeholder="Tracker">
                </div>
              </div>
            </form>
          </div>

          <!-- OMNI BAR -->
          <div class="omni ui fluid icon input">
            <input placeholder="Enter search query, magnet URI, torrent URL or drop a torrent file here"
              v-model="omni.inputs.omni" @input="$emit('parse-omni')" @keydown.enter="$emit('submit-omni')" />
            <div class="icon-wrapper" @click="uploadTorrent">
              <i class="icon" :class="{search: omni.mode.search, magnet: omni.mode.magnet || omni.mode.torrent, upload: !omni.mode.search && !omni.mode.magnet && !omni.mode.torrent}"></i>
            </div>
          </div>

          <!-- MAGNET FIELD ERROR -->
          <div v-if="omni.omnierr" class="ui error message">
            <p>{{omni.omnierr}}</p>
          </div>

          <!-- START TORRENT BUTTONS -->
          <div class="search buttons" v-if="omni.mode.torrent">
            <div @click="$emit('submit-omni')" class="ui tiny blue button" :class="{loading: apiing, disabled: apiing }">
              <span>Start Torrent</span>
            </div>
          </div>

          <!-- SEARCH BUTTONS -->
          <div class="search buttons" v-if="omni.mode.search && omni.inputs.provider">
            <select class="ui green button" v-model="omni.inputs.provider">
              <option v-for="(s, id) in omni.providers" :key="id" :value="id">{{ s.name }}</option>
            </select>
            <div @click="$emit('submit-search')" class="ui tiny blue button" :class="{loading: searching,
                disabled: omni.noResults || omni.hasMore && omni.results && omni.results.length > 0
              }">
              <span v-if="omni.noResults">No results!</span>
              <span v-else>Search</span>
            </div>
          </div>

          <div class="ui error message" v-if="omni.mode.search && !omni.inputs.provider">
            <p>You have no search providers</p>
          </div>

          <!-- SEARCH RESULTS -->
          <div class="results" v-if="omni.mode.search && omni.results && omni.results.length > 0">
            <table class="ui unstackable compact striped tcld table">
              <tr v-for="r in omni.results" :key="r.name">
                <td class="name">
                  <a :href="r.url" target="_blank">{{ r.name }}</a>
                </td>
                <td class="size" v-if="r.size">{{ r.size }}</td>
                <td class="users">
                  <span class="seeds">{{ r.seeds }}</span>
                  <br/>
                  <span class="peers"> {{ r.peers }}</span>
                </td>
                <td class="controls">
                  <i @click="$emit('submit-search-item', r)" class="ui green download icon"></i>
                </td>
              </tr>
              <tr v-if="omni.hasMore">
                <td class="loadmore" colspan="10">
                  <div @click="$emit('submit-search')" class="ui tiny blue button" :class="{loading: searching}">
                    Load more
                  </div>
                </td>
              </tr>
            </table>
          </div>

          <!-- MAGNET BUTTONS -->
          <div class="magnet buttons" v-if="omni.mode.magnet">
            <div @click="$emit('submit-omni')" class="ui tiny blue button" :class="{loading: apiing}">
              Load Magnet
            </div>
            <div @click="omni.edit = !omni.edit" :class="{green: omni.edit}" class="ui tiny button">Edit</div>
          </div>
        </section>
      `,
      props: ['state', 'omni', 'apiing', 'searching'],
      watch: {
        'omni.inputs.provider'(p) {
          if (p) window.localStorage.setItem('tcProvider', p);
          this.$emit('parse-omni');
        }
      },
      methods: {
        parseMagnetString() {
          this.omni.omnierr = null;
          if (!/^[A-Za-z0-9]+$/.test(this.omni.magnet.infohash)) {
            this.omni.omnierr = "Invalid Info Hash";
            return;
          }
          
          const cleanTrackers = this.omni.magnet.trackers.filter(t => t.v);
          this.omni.inputs.omni = "magnet:?" +
            "xt=urn:btih:" + (this.omni.magnet.infohash || "") +
            "&dn=" + (this.omni.magnet.name || "").replace(/\W/g, "").replace(/\s+/g, "+") +
            cleanTrackers.map(t => "&tr=" + encodeURIComponent(t.v)).join("");
          
          this.omni.magnet.trackers = [...cleanTrackers, { v: "" }];
          this.$emit('parse-omni');
        },

        uploadTorrent() {
          // File upload functionality would need to be implemented
          console.log('Upload torrent clicked');
        }
      }
    },

    'torrents-section': {
      template: `
        <section class="torrents">
          <div class="section-header">
            <h3 class="ui header">
              Torrents
            </h3>
            <h5 class="right">
              {{ numKeys(state.Torrents) }} torrent{{ numKeys(state.Torrents) == 1 ? '' : 's' }}
            </h5>
          </div>

          <div v-if="isEmpty(state.Torrents)" class="ui message nodownloads">
            <p>Add torrents above</p>
          </div>

          <div v-for="(t, hash) in state.Torrents" :key="hash" :class="{open: t.open}" class="ui torrent segment">

            <div v-if="!t.Loaded" class="ui active inverted dimmer">
              <div class="ui text loader">Loading</div>
            </div>

            <div class="ui stackable grid">
              <div class="two column row">
                <div class="info column">
                  <div class="name">
                    <span v-if="!ready(t.Name+'.zip')">
                      {{ t.Name }}
                    </span>
                    <a v-else :href="ready(t.Name+'.zip').url">
                      {{ t.Name }}
                    </a>
                  </div>
                  <div class="hash">#{{ t.InfoHash }}</div>
                  <div class="ui blue progress" :class="{active: t.Percent > 0 && t.Percent < 100}">
                    <div class="bar" :style="{width: t.Percent + '%'}">
                      <div class="progress"></div>
                    </div>
                  </div>
                </div>
                <div class="controls column">
                  <div>
                    <div class="ui mini buttons">
                      <a class="ui button" :class="{blue: t.$showFiles}" @click="t.$showFiles = !t.$showFiles">
                        <i class="file icon"></i> Files
                      </a>
                      <a :disabled="t.Started" class="ui button" :class="{green: !t.Started}" @click="$emit('submit-torrent', 'start', t)">
                        <i class="cloud download icon"></i> Start
                      </a>
                      <a v-if="t.Started" class="ui red button" @click="$emit('submit-torrent', 'stop', t)">
                        <i class="stop icon"></i> Stop
                      </a>
                      <a v-if="!t.Started" class="ui red button" style="z-index: 99999;" @click="$emit('submit-torrent', 'delete', t)">
                        <span v-if="!t.Loaded">
                          <i class="ban icon"></i> Cancel</span>
                        <span v-else>
                          <i class="trash icon"></i> Remove</span>
                      </a>
                    </div>
                  </div>

                  <div v-if="t.Started" class="status download">
                    <span :class="{muted:t.Downloaded == 0}">{{formatBytes(t.Downloaded)}}</span>
                    <span> / {{formatBytes(t.Size)}}</span>
                    <span> - {{t.Percent }}% </span>
                    <span style="font-weight:bold" :class="{muted:t.DownloadRate == 0}"> - {{formatBytes(t.DownloadRate)}}/s</span>
                  </div>
                </div>
              </div>
              <div class="row" v-if="t.$showFiles && t.Loaded">
                <div class="column">
                  <table class="ui unstackable compact striped downloads tcld table">
                    <thead>
                      <tr>
                        <th class="name">File</th>
                        <th class="size">Size</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="!t.Files || t.Files.length == 0">
                        <td colspan="2" class="muted">No files</td>
                      </tr>
                      <tr class="download file" v-for="f in sortedFiles(t.Files)" :key="f.Path">
                        <td class="name">
                          <div>
                            <span>{{ filename(f.Path) }}</span>
                            <span class="percent" v-if="f.Percent > 0 && f.Percent < 100">{{ f.Percent }}% </span>
                            <div v-if="f.Percent > 0 && f.Percent < 100" class="ui blue active progress">
                              <div class="bar" :style="{width: f.Percent + '%'}">
                                <div class="progress"></div>
                              </div>
                            </div>
                          </div>
                          <div v-if="f.downloadError" class="ui error message">
                            <i class="close icon" @click="f.downloadError = null"></i>
                            <p>{{f.downloadError}}</p>
                          </div>
                        </td>
                        <td class="size">
                          {{ formatBytes(f.Size) }}
                          <i v-if="f.Percent == 100" class="green check icon"></i>
                        </td>
                      </tr>
                    </tbody>
                    <tfoot v-if="numKeys(t.Files) > 1">
                      <tr>
                        <th class="name">
                          {{ numKeys(t.Files) }} Files
                        </th>
                        <th>
                          {{ formatBytes(t.Size) }} Total
                        </th>
                      </tr>
                    </tfoot>
                  </table>
                </div>
              </div>
            </div>
          </div>
        </section>
      `,
      props: ['state', 'torrents'],
      methods: {
        numKeys(obj) {
          return obj ? Object.keys(obj).length : 0;
        },
        isEmpty(obj) {
          return this.numKeys(obj) === 0;
        },
        ready(f) {
          const path = typeof f === "object" ? f.path : f;
          return this.state.Uploads && this.state.Uploads[path];
        },
        formatBytes(n, d = 1) {
          if (typeof n !== "number" || isNaN(n) || n == 0) return "0 B";
          const i = Math.floor(Math.floor(Math.log(n) * Math.LOG10E) / 3);
          const f = Math.pow(10, d);
          const s = Math.round(n / Math.pow(10, i * 3) * f) / f;
          return s.toString().replace(/\.0+$/, "") + " " + ["", "K", "M", "G", "T", "P", "Z"][i] + "B";
        },
        filename(path) {
          return /\/([^\/]+)$/.test(path) ? RegExp.$1 : path;
        },
        sortedFiles(files) {
          if (!files) return [];
          return files.slice().sort((a, b) => a.Path.localeCompare(b.Path));
        }
      }
    },

    'downloads-section': {
      template: `
        <section class="downloads">
          <div class="section-header">
            <h3 class="ui header">
              Downloads
            </h3>
            <h5 class="right" v-if="state.Stats && state.Stats.System && state.Stats.System.set">
              {{ formatBytes(state.Stats.System.diskTotal - state.Stats.System.diskUsed) }} free
            </h5>
          </div>

          <div v-if="numDownloads() == 0" class="ui message nodownloads">
            <p>Download files above</p>
          </div>

          <div v-if="numDownloads() > 0" class="ui segment">
            <div class="ui list">
              <download-node v-for="node in state.Downloads.Children" :key="node.Name" :node="node" :state="state" :parent-path="''"></download-node>
            </div>
          </div>
        </section>
      `,
      props: ['state', 'downloads'],
      methods: {
        numDownloads() {
          if (this.state.Downloads && this.state.Downloads.Children)
            return this.state.Downloads.Children.length;
          return 0;
        },
        formatBytes(n, d = 1) {
          if (typeof n !== "number" || isNaN(n) || n == 0) return "0 B";
          const i = Math.floor(Math.floor(Math.log(n) * Math.LOG10E) / 3);
          const f = Math.pow(10, d);
          const s = Math.round(n / Math.pow(10, i * 3) * f) / f;
          return s.toString().replace(/\.0+$/, "") + " " + ["", "K", "M", "G", "T", "P", "Z"][i] + "B";
        }
      },
      components: {
        'download-node': {
          template: `
            <div class="item">
              <i class="wrapper icon">
                <i v-if="isdir()" :class="icon()" @click="toggle()"></i>
                <i v-else :class="icon()"></i>
              </i>
              
              <div class="content">
                <div class="header">
                  <a v-if="!isdownloading()" :href="'download/' + node.$path">{{ node.Name }}</a>
                  <span v-else>{{ node.Name }}</span>
                  <span v-if="!isdownloading()" class="controls">
                    <i v-if="!confirm" @click="preremove()" class="red trash icon"></i>
                    <i v-if="!deleting && confirm" @click="deleting = true; remove();" class="red check icon"></i>
                    <i v-if="deleting" class="grey notched circle loading icon"></i>
                    <i v-if="imagePreview || videoPreview || audioPreview" @click="togglePreview()" :class="'blue ' + (showPreview ? 'circle outline' : 'video play outline') + ' icon'"></i>
                  </span>
                </div>
                <div class="description">{{ formatBytes(node.Size) }} updated {{ ago(node.Modified) }}</div>
                <div class="preview" v-if="showPreview">
                  <audio v-if="audioPreview" controls>
                    <source :src="showPreview ? ('download/'+node.$path) : ''">
                  </audio>
                  <img v-if="imagePreview" :src="showPreview ? ('download/'+node.$path) : ''">
                  <video controls autoplay v-if="videoPreview">
                    <source :src="showPreview ? ('download/'+node.$path) : ''">
                  </video>
                </div>
                <div class="list" v-if="isdir() && !closed()">
                  <download-node v-for="child in node.Children" :key="child.Name" :node="child" :state="state" :parent-path="node.$path"></download-node>
                </div>
              </div>
            </div>
          `,
          props: ['node', 'state', 'parentPath'],
          data() {
            return {
              confirm: false,
              deleting: false,
              showPreview: false
            };
          },
          computed: {
            audioPreview() { return /\.(mp3|m4a)$/.test(this.node.$path); },
            imagePreview() { return /\.(jpe?g|png|gif)$/.test(this.node.$path); },
            videoPreview() { return /\.(mp4|mkv|mov)$/.test(this.node.$path); }
          },
          mounted() {
            const pathArray = [this.node.Name];
            if (this.parentPath) {
              pathArray.unshift(this.parentPath);
              this.node.$depth = (this.parentPath.split('/').length) + 1;
            } else {
              this.node.$depth = 1;
            }
            this.node.$path = pathArray.join("/");
            this.node.$closed = this.agoHrs(this.node.Modified) > 24;

            // Search for this file in torrents
            const torrents = this.state.Torrents;
            if (this.isfile() && torrents) {
              for (let ih in torrents) {
                const torrent = torrents[ih];
                const files = torrent.Files;
                if (files) {
                  for (let i = 0; i < files.length; i++) {
                    const f = files[i];
                    if (f.Path === this.node.$path) {
                      this.node.$torrent = torrent;
                      this.node.$file = f;
                      break;
                    }
                  }
                }
                if (this.node.$file) break;
              }
            }
          },
          methods: {
            isfile() { return !this.node.Children; },
            isdir() { return !this.isfile(); },
            closed() { return this.node.$closed; },
            toggle() { this.node.$closed = !this.node.$closed; },
            isdownloading() {
              return this.node.$torrent && this.node.$torrent.Loaded && 
                     this.node.$torrent.Started && this.node.$file && this.node.$file.Percent < 100;
            },
            preremove() {
              this.confirm = true;
              setTimeout(() => { this.confirm = false; }, 3000);
            },
            remove() {
              axios.delete("download/" + this.node.$path);
            },
            togglePreview() {
              this.showPreview = !this.showPreview;
            },
            icon() {
              const c = [];
              if (this.isdownloading()) {
                c.push("spinner", "loading");
              } else {
                c.push("outline");
                if (this.isfile()) {
                  if (this.audioPreview) c.push("audio");
                  else if (this.imagePreview) c.push("image");
                  else if (this.videoPreview || /\.(avi)$/.test(this.node.$path)) c.push("video");
                  c.push("file");
                } else {
                  c.push("folder");
                  if (!this.closed()) c.push("open");
                }
              }
              c.push("icon");
              return c.join(" ");
            },
            formatBytes(n, d = 1) {
              if (typeof n !== "number" || isNaN(n) || n == 0) return "0 B";
              const i = Math.floor(Math.floor(Math.log(n) * Math.LOG10E) / 3);
              const f = Math.pow(10, d);
              const s = Math.round(n / Math.pow(10, i * 3) * f) / f;
              return s.toString().replace(/\.0+$/, "") + " " + ["", "K", "M", "G", "T", "P", "Z"][i] + "B";
            },
            ago(t) {
              return moment(t).fromNow();
            },
            agoHrs(t) {
              return moment().diff(moment(t), "hours");
            }
          }
        }
      }
    }
  }
}).mount('#app');
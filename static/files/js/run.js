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
          omni: window.localStorage.tcOmni || "",
          provider: window.localStorage.tcProvider || "tpb"
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
          this.api.torrentfile(data);
        };
      });
    },

    // API methods
    api: {
      request(action, data) {
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
      configure: (data) => this.api.request("configure", data),
      magnet: (data) => this.api.request("magnet", data),
      url: (data) => this.api.request("url", data),
      torrent: (data) => this.api.request("torrent", data),
      file: (data) => this.api.request("file", data),
      torrentfile: (data) => this.api.request("torrentfile", data)
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
      this.api.configure(data);
    },

    submitTorrent(action, t) {
      this.api.torrent([action, t.InfoHash].join(":"));
    },

    submitFile(action, t, f) {
      this.api.file([action, t.InfoHash, f.Path].join(":"));
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
        this.api.url(this.omni.inputs.omni);
      } else if (this.omni.mode.magnet) {
        this.api.magnet(this.omni.inputs.omni);
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
        this.api.magnet(result.magnet);
        return;
      } else if (result.torrent) {
        this.api.url(result.torrent);
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
            this.api.url(data.torrent);
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
          this.api.magnet(magnet);
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
      template: `<section class="omni">Omni section placeholder</section>`,
      props: ['state', 'omni', 'apiing', 'searching']
    },

    'torrents-section': {
      template: `<section class="torrents">Torrents section placeholder</section>`,
      props: ['state', 'torrents']
    },

    'downloads-section': {
      template: `<section class="downloads">Downloads section placeholder</section>`,
      props: ['state', 'downloads']
    }
  }
}).mount('#app');
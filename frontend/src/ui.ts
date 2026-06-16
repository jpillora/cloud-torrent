import { reactive } from 'vue'

// ui holds cross-component view state that is NOT part of the synced server
// State. It is the Vue equivalent of master's `$rootScope` flags:
//   - requests:   number of API calls currently in flight (drives the header
//                 request spinner; maintained by api.ts).
//   - configEdit: whether the config editor is open (toggled by the header
//                 settings icon; read by ConfigEditor).
//   - omniEdit:   whether the Omni magnet editor is open (toggled by both the
//                 header magnet icon and Omni's own Edit button).
export const ui = reactive({
  requests: 0,
  configEdit: false,
  omniEdit: false,
})

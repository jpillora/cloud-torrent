package server

import "net/http"

func (s *Server) serveFiles(w http.ResponseWriter, r *http.Request) {
	// if strings.HasPrefix(r.URL.Path, "/download/") {
	// 	url := strings.TrimPrefix(r.URL.Path, "/download/")
	// 	//dldir is absolute
	// 	dldir := s.state.Config.DownloadDirectory
	// 	file := filepath.Join(dldir, url)
	// 	//only allow fetches/deletes inside the dl dir
	// 	if !strings.HasPrefix(file, dldir) || dldir == file {
	// 		http.Error(w, "Nice try\n"+dldir+"\n"+file, http.StatusBadRequest)
	// 		return
	// 	}
	// 	info, err := os.Stat(file)
	// 	if err != nil {
	// 		http.Error(w, "File stat error: "+err.Error(), http.StatusBadRequest)
	// 		return
	// 	}
	// 	switch r.Method {
	// 	case "GET":
	// 		f, err := os.Open(file)
	// 		if err != nil {
	// 			http.Error(w, "File open error: "+err.Error(), http.StatusBadRequest)
	// 			return
	// 		}
	// 		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	// 		f.Close()
	// 	case "DELETE":
	// 		if err := os.RemoveAll(file); err != nil {
	// 			http.Error(w, "Delete failed: "+err.Error(), http.StatusInternalServerError)
	// 		}
	// 	default:
	// 		http.Error(w, "Not allowed", http.StatusMethodNotAllowed)
	// 	}
	// 	return
	// }
	s.static.ServeHTTP(w, r)
}

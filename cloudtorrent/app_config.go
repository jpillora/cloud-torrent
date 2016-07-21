package cloudtorrent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"

	"golang.org/x/net/context"
)

type AppConfig struct {
	User, Pass string
	Title      string
}

var EmptyConfig = json.RawMessage("{}")

//file system state
type FileSystemState struct {
	Enabled bool           `json:",omitempty"`
	Syncing bool           `json:",omitempty"`
	Config  interface{}    `json:",omitempty"`
	Root    json.Marshaler `json:",omitempty"`
	Error   string         `json:",omitempty"`
}

func (a *App) handleConfigure(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	cfgs := rawMessages{}
	if err := json.NewDecoder(r.Body).Decode(&cfgs); err != nil {
		return errors.New("JSON error")
	}
	if err := a.configureAll(cfgs); err != nil {
		return err
	}
	return nil
}

func (a *App) configureApp(raw json.RawMessage) (interface{}, error) {
	if err := json.Unmarshal(raw, &a.config); err != nil {
		return nil, err
	}
	if a.Title == "" {
		a.Title = "Cloud Torrent"
	}
	return &a.config, nil
}

func (a *App) configureAllRaw(b []byte) error {
	cfgs := rawMessages{}
	if err := json.Unmarshal(b, &cfgs); err != nil {
		return fmt.Errorf("initial configure failed: %s", err)
	}
	return a.configureAll(cfgs)
}

func (a *App) configureAll(cfgs rawMessages) error {
	changed := false
	for name, raw := range cfgs {
		//normalize raw
		indented := bytes.Buffer{}
		if err := json.Indent(&indented, raw, "", "  "); err != nil {
			panic(err)
		}
		r := indented.Bytes()
		//check for fs
		f, ok := a.fileSystems[name]
		//validate name
		if name != "App" && !ok {
			continue
		}
		//compare to last update
		prev := a.prevConfigs[name]
		if bytes.Equal(prev, r) {
			continue
		}
		//apply!
		var v interface{}
		var err error
		if name == "App" {
			v, err = a.configureApp(r)
		} else {
			v, err = f.Configure(r)
		}
		if err != nil {
			if bytes.Equal(raw, EmptyConfig) {
				continue
			}
			logf("[%s] configuration error: %s", name, err)
			return err
		}
		//note successful configure
		a.state.Configurations[name] = v
		a.prevConfigs[name] = r
		changed = true
		//first config? start syncing filesystems
		if state, ok := a.state.FSS[name]; ok && !state.Syncing {
			state.Syncing = true
			a.state.Push()
			a.startFSSync(f)
		}
	}
	if changed {
		//write back to disk if changed
		b, _ := json.MarshalIndent(&cfgs, "", "  ")
		ioutil.WriteFile(a.ConfigPath, b, 0600)
		//update frontend
		a.state.Push()
		logf("reconfigured")
	}
	return nil
}

//rawMessages allows json marshalling of string->raw
type rawMessages map[string]json.RawMessage

func (m rawMessages) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	keys := make([]string, len(m))
	i := 0
	for k, _ := range m {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	//manually write object
	buf.WriteString("{")
	for i, k := range keys {
		buf.WriteString(`"`)
		buf.WriteString(k)
		buf.WriteString(`":`)
		buf.Write(m[k])
		if i < len(keys)-1 {
			buf.WriteRune(',')
		}
	}
	buf.WriteString("}")
	return buf.Bytes(), nil
}

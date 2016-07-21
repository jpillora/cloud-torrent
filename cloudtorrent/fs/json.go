package fs

import (
	"encoding/json"
	"log"
	"time"
)

type JSONNode struct {
	Node
}

func (j *JSONNode) MarshalJSON() ([]byte, error) {
	i, err := j.Node.Stat()
	if err != nil {
		log.Printf("[json node] stat err: %s", err)
		return nil, err
	}
	s := struct {
		Name     string
		Size     int64       `json:",omitempty"`
		MTime    *time.Time  `json:",omitempty"`
		IsDir    bool        `json:",omitempty"`
		Children []*JSONNode `json:",omitempty"`
	}{
		Name:     j.Name(),
		Size:     i.Size(),
		IsDir:    i.IsDir(),
		Children: nil,
	}
	if t := i.ModTime(); !t.IsZero() {
		s.MTime = &t
	}
	c := j.Children()
	if l := len(c); l > 0 {
		s.Children = make([]*JSONNode, l)
		for i, n := range c {
			s.Children[i] = &JSONNode{n}
		}
	}
	b, err := json.Marshal(&s)
	if err != nil {
		log.Printf("[json node] struct err: %s", err)
		return nil, err
	}
	return b, nil
}

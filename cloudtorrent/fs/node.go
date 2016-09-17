package fs

import (
	"encoding/json"
	"log"
	"os"
	"reflect"
	"strings"
	"time"
)

type Foo struct {
	BaseNode
}

//1. edit node tree. insert. delete
//2. jsonify node tree
//3. extensible

type Node interface {
	Name() string
	Get(path string) Node
	GetChildren() []Node
	Upsert(path string, child Node) bool
	Delete(path string) bool
	json.Marshaler
}

func NewBaseNode(name string) *BaseNode {
	return &BaseNode{
		Children: map[string]Node{},
		BaseInfo: BaseInfo{Name: name},
	}
}

type BaseNode struct {
	Children map[string]Node
	BaseInfo
}

type BaseInfo struct {
	Name  string
	Size  int64
	IsDir bool
	MTime time.Time
}

func (b *BaseNode) childmap(n Node) map[string]Node {
	return b.Children
}

func childmap(n Node) map[string]Node {
	mapper, ok := n.(interface {
		childmap() map[string]Node
	})
	if !ok {
		return nil
	}
	return mapper.childmap()
}

func (n *BaseNode) get(path string, mkdirp bool) (node Node, parent Node) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 {
		return nil, nil
	}
	//find/initialise all parent nodes
	parent = n
	name := parts[0]
	parents := parts[1:]
	for _, pname := range parents {
		p, ok := n.Children[pname]
		if ok {
			parent = p
		} else if mkdirp {
			//create missing parent
			b := NewBaseNode(pname)
			b.BaseInfo.IsDir = true
			parent = b
			n.Children[pname] = b
		} else {
			return nil, nil
		}
	}
	m := childmap(parent)
	if m == nil {
		return nil, parent
	}
	//get child and parent node
	return m[name], parent
}

func (n *BaseNode) Get(path string) Node {
	node, _ := n.get(path, false)
	return node
}

func (n *BaseNode) Upsert(path string, child Node) bool {
	existing, parent := n.get(path, true)
	m := childmap(parent)
	if m == nil {
		return false //shouldnt happen
	}
	m[child.Name()] = child
	return reflect.DeepEqual(existing, child)
}

func (n *BaseNode) Delete(path string) bool {
	existing, parent := n.get(path, false)
	if existing == nil {
		return false
	}
	m := childmap(parent)
	if m == nil {
		return false //shouldnt happen
	}
	delete(m, existing.Name())
	return true
}

func (n *BaseNode) GetChildren() []Node {
	nodes := make([]Node, len(n.Children))
	i := 0
	for _, node := range n.Children {
		nodes[i] = node
		i++
	}
	return nodes
}

func (n *BaseNode) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"Name": n.BaseInfo.Name,
	}
	if n.BaseInfo.IsDir {
		m["IsDir"] = true
	}
	if n.BaseInfo.Size > 0 {
		m["Size"] = n.BaseInfo.Size
	}
	if !n.MTime.IsZero() {
		m["MTime"] = n.BaseInfo.MTime
	}
	if c := n.GetChildren(); len(c) > 0 {
		m["Children"] = c
	}
	return m
}

func (n *BaseNode) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(n.ToMap())
	if err != nil {
		log.Printf("base node marshal: %s", err)
	}
	return b, err
}

func filename(path string) string {
	for i := len(path) - 2; i >= 0; i-- {
		if path[i] == os.PathSeparator {
			return string(path[i+1:])
		}
	}
	return path
}

// Name of the file.
func (b *BaseNode) Name() string {
	return b.BaseInfo.Name
}

// Size of the file.
func (b *BaseNode) Size() int64 {
	return b.BaseInfo.Size
}

// IsDir returns true if the file is a directory.
func (b *BaseNode) IsDir() bool {
	return b.BaseInfo.IsDir
}

// Sys is not implemented.
func (b *BaseNode) Sys() interface{} {
	return nil
}

// ModTime returns the modification time.
func (b *BaseNode) ModTime() time.Time {
	return b.BaseInfo.MTime
}

// Mode returns the file mode flags.
func (b *BaseNode) Mode() os.FileMode {
	var m os.FileMode
	if b.BaseInfo.IsDir {
		m |= os.ModeDir
	}
	return m
}

package storage

import "time"

//Fs provides a common file-system API
type Fs interface {
	Configure(interface{}) error
	List(basePath string) (*Node, error)
}

type Node struct {
	Name     string
	Size     int64
	Modified time.Time
	Children []*Node
}

package fs

type Node interface {
	Name() string
	Children() []Node
}

type BasicNode struct {
	name     string
	children []Node
}

func (n *BasicNode) Name() string {
	return n.name
}

func (n *BasicNode) Children() []Node {
	return n.children
}

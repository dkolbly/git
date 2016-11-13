package git

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type Node struct {
	Name string
	Perm uint
	Ref  Ptr
}

func (n *Node) IsDir() bool {
	return n.Perm&040000 != 0
}

type Tree struct {
	repo     *Git
	name     Ptr
	raw      []byte
	list     []string
	contents map[string]*Node
}

func (t *Tree) Listing() []string {
	return t.list
}

func (t *Tree) Walk(path string) *Node {
	components := strings.Split(path, "/")
	num := len(components)

	at := t
	for _, comp := range components[:num-1] {
		fmt.Printf("indirect %q\n", comp)
		n := at.contents[comp]
		if n == nil {
			fmt.Printf("No entry %q\n", comp)
			return nil
		}
		if !n.IsDir() {
			// not a subdir; this is a user error, attempting to deref
			// past a non-dir
			fmt.Printf("%q not a dir\n", comp)
			return nil
		}
		o := t.repo.Get(&n.Ref)
		if o == nil {
			// bad Ref!
			panic("bad ref")
			return nil
		}
		o, err := o.Load()
		if err != nil {
			panic(err)
			return nil
		}

		if o == nil {
			panic("could not load")
			return nil
		}
		if t2, ok := o.(*Tree); ok {
			at = t2
		} else {
			// not a tree... but it was marked IsDir(), so this
			// is really an internal error
			panic("dir is not a tree")
			return nil
		}
	}
	fmt.Printf("at %#v\n", at)
	return at.contents[components[num-1]]

}

func (t *Tree) Load() (GitObject, error) {
	return t, nil
}

func (t *Tree) Name() *Ptr {
	return &t.name
}

func (t *Tree) Type() ObjType {
	return ObjTree
}

func (t *Tree) Payload() ([]byte, error) {
	return t.raw, nil
}

func (g *Git) loadTree(name *Ptr, buf []byte) (*Tree, error) {
	t := &Tree{
		name:     *name,
		repo:     g,
		raw:      buf,
		contents: make(map[string]*Node),
	}
	r := bytes.NewBuffer(buf)
	for {
		fmt.Printf("----\n")
		mode, err := r.ReadBytes(' ')
		if err != nil {
			break
		}
		fmt.Printf("Mode <%s>\n", mode[:len(mode)-1])
		perm, err := strconv.ParseUint(string(mode[:len(mode)-1]), 8, 32)

		file, err := r.ReadBytes(0)
		if err != nil {
			break
		}
		fmt.Printf("File <%s>\n", file[:len(file)-1])
		node := &Node{
			Name: string(file[:len(file)-1]),
			Perm: uint(perm),
		}

		r.Read(node.Ref.hash[:])
		fmt.Printf("Addr <%s>\n", &node.Ref)
		t.list = append(t.list, node.Name)
		t.contents[node.Name] = node
	}
	return t, nil
}

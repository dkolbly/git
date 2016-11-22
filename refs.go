package git

import (
	"io/ioutil"
	"path"
)

func (g *Git) Branch(name string) (*Ptr, error) {
	return g.readRef(path.Join(g.Dir, "refs/heads", name))
}

func (g *Git) Tag(name string) (*Ptr, error) {
	return g.readRef(path.Join(g.Dir, "refs/tags", name))
}

func (g *Git) readRef(file string) (*Ptr, error) {
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	// we expect it to be 41 bytes, which is a SHA1 + '\n'
	if len(buf) != 41 {
		return nil, ErrInvalidRef
	}
	if buf[40] != '\n' {
		return nil, ErrInvalidRef
	}
	p, ok := ParsePtr(string(buf[:40]))
	if ok {
		return &p, nil
	}
	return nil, ErrInvalidRef
}

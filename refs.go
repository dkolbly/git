package git

import (
	"io/ioutil"
	"os"
	"path"
)

func (g *Git) Branch(name string) (*Ptr, error) {
	nr, err := g.readRef(Head, name)
	if err != nil {
		return nil, err
	}
	return &nr.Ptr, nil
}

func (g *Git) Tag(name string) (*Ptr, error) {
	nr, err := g.readRef(Tag, name)
	if err != nil {
		return nil, err
	}
	return &nr.Ptr, nil
}

func (g *Git) readRef(t RefType, name string) (*NamedRef, error) {
	for _, store := range g.stores {
		nr := store.GetNamed(t, name)
		if nr != nil {
			return nr, nil
		}
	}
	switch t {
	case Head:
		return nil, ErrNoBranch
	case Tag:
		return nil, ErrNoTag
	default:
		panic("invalid ref type")
	}
}

type RefType int

const (
	Head = RefType(iota)
	Tag
)

func (t RefType) String() string {
	switch t {
	case Head:
		return "heads"
	case Tag:
		return "tags"
	default:
		panic("unknown reftype")
	}
}

type NamedRef struct {
	Ptr     Ptr
	RefType RefType
	Name    string
}

func (g *GitDir) NameEnumerate(t RefType) ([]NamedRef, error) {
	return g.walkLinks(t, path.Join(g.Dir, "refs", t.String()))
}

func (g *GitDir) walkLinks(t RefType, refdir string) ([]NamedRef, error) {
	ret := []NamedRef{}
	var recur func(string) error

	recur = func(pre string) error {
		d := path.Join(refdir, pre)
		fi, err := ioutil.ReadDir(d)
		if err != nil {
			log.Error("Error walking refs: %s", err)
			return err
		}
		for _, entry := range fi {
			n := entry.Name()
			refname := path.Join(pre, n)
			if entry.IsDir() {
				err := recur(refname)
				if err != nil {
					return err
				}
			} else {
				ptr, err := g.readRef(path.Join(d, n))
				if err != nil {
					log.Error("could not read ref %s", err)
					return err
				}
				nr := NamedRef{
					RefType: t,
					Name:    refname,
					Ptr:     *ptr,
				}
				ret = append(ret, nr)
			}
		}
		return nil
	}
	err := recur("")
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (g *GitDir) Branches() ([]NamedRef, error) {
	return g.walkLinks(Head, "heads")
}

func (g *GitDir) Tags() ([]NamedRef, error) {
	return g.walkLinks(Tag, "tags")
}

func (g *GitDir) GetNamed(t RefType, name string) *NamedRef {
	ptr, err := g.readRef(path.Join(g.Dir, "refs", t.String(), name))
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warning("Failed to read %s/%s: %s", t, name, err)
		}
		return nil
	}
	return &NamedRef{
		Ptr:     *ptr,
		RefType: t,
		Name:    name,
	}
}

func (g *GitDir) readRef(file string) (*Ptr, error) {
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

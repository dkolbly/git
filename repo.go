package git

import (
	"encoding/hex"
	"errors"
	"github.com/dkolbly/logging"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

var ErrNoBranch = errors.New("no such branch")
var ErrNoTag = errors.New("no such tag")

var log = logging.New("git")

type Git struct {
	stores []Store
}

func New() *Git {
	return &Git{}
}

func (g *Git) AddStore(s Store) {
	g.stores = append(g.stores, s)
}

type Store interface {
	GetNamed(RefType, string) *NamedRef
	Get(obj *Ptr) GitObject
	EnumerateTo(chan<- Ptr)
}

// optional interface
type NameEnumerater interface {
	NameEnumerate(t RefType) ([]NamedRef, error)
}

func Open(d string) (*Git, error) {
	g := &Git{}
	Bare(g, d)

	lst, err := ioutil.ReadDir(path.Join(d, "objects/pack"))

	if err == nil {
		for _, f := range lst {
			if strings.HasSuffix(f.Name(), ".pack") {
				pfile := path.Join(d, "objects/pack", f.Name())
				_, err := IncludePackFile(g, pfile)
				if err != nil {
					log.Error("Rats: %s", err)
				}
			}
		}
	}

	return g, nil
}

func (g *Git) Get(p *Ptr) GitObject {
	for _, store := range g.stores {
		o := store.Get(p)
		if o != nil {
			return o
		}
	}
	return nil
}

func (g *Git) Enumerate() <-chan Ptr {
	ch := make(chan Ptr, 10000)

	go g.enumerateTo(ch)
	return ch
}

func (g *Git) enumerateTo(to chan<- Ptr) {
	defer close(to)

	for _, store := range g.stores {
		store.EnumerateTo(to)
	}
}

func (p *PackFile) EnumerateTo(to chan<- Ptr) {
	for _, item := range p.indexContents {
		to <- item
	}
}

func (g *GitDir) EnumerateTo(to chan<- Ptr) {

	// walk the loose objects
	f, err := os.Open(path.Join(g.Dir, "objects"))
	if err != nil {
		return
	}
	defer f.Close()

	lst, err := f.Readdirnames(-1)
	if err != nil {
		return
	}
	for _, major := range lst {
		if len(major) != 2 {
			continue
		}
		b0, err := hex.DecodeString(major)
		if err != nil || len(b0) != 1 {
			continue
		}
		sub, err := os.Open(path.Join(g.Dir, "objects", major))
		if err != nil {
			continue
		}
		sublst, err := sub.Readdirnames(-1)
		sub.Close()
		if err != nil {
			continue
		}
		for _, minor := range sublst {
			b1, err := hex.DecodeString(minor)
			if err == nil && len(b1) == 19 {
				var p Ptr
				p.hash[0] = b0[0]
				copy(p.hash[1:], b1)
				to <- p
			}
		}
	}
}

func (g *Git) enumNamed(t RefType) ([]NamedRef, error) {
	var lst []NamedRef
	for _, store := range g.stores {
		if ne, ok := store.(NameEnumerater); ok {
			more, err := ne.NameEnumerate(t)
			if err != nil {
				return nil, err
			}
			lst = append(lst, more...)
		}
	}
	return lst, nil
}

func (g *Git) Branches() ([]NamedRef, error) {
	return g.enumNamed(Head)
}

func (g *Git) Tags() ([]NamedRef, error) {
	return g.enumNamed(Tag)
}

package git

import (
	"encoding/hex"
	"github.com/dkolbly/logging"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

var log = logging.New("git")

type Git struct {
	Dir   string
	packs []*PackFile
}

func Open(d string) (*Git, error) {
	g := &Git{
		Dir: d,
	}
	lst, err := ioutil.ReadDir(path.Join(d, "objects/pack"))

	if err == nil {
		for _, f := range lst {
			if strings.HasSuffix(f.Name(), ".pack") {
				p, err := g.Unpack(path.Join("objects/pack", f.Name()))
				if err == nil {
					//fmt.Printf("  including pack %s\n", f.Name())
					g.packs = append(g.packs, p)
				} /*else {
					fmt.Printf("  %s failed: %s\n", f.Name(), err)
				}*/
			}
		}
	}

	return g, nil
}

func (g *Git) Get(p *Ptr) GitObject {
	// check disk
	o := g.getLoose(p)
	if o != nil {
		return o
	}
	// check packs
	for _, pack := range g.packs {
		o := pack.Get(p)
		if o != nil {
			return o
		}
	}
	return nil
}

/*func (g *Git) Get(p *Ptr) (GitObject, error) {
	// check disk
	loose := path.Join(g.Dir, "objects", h[0:2], h[2:])
	fi, err := os.Stat(
}
*/

func (g *Git) Enumerate() <-chan Ptr {
	ch := make(chan Ptr, 10000)

	go g.enumerateTo(ch)
	return ch
}

func (g *Git) enumerateTo(to chan<- Ptr) {
	defer close(to)

	// walk the packs
	for _, pack := range g.packs {
		for _, item := range pack.indexContents {
			to <- item
		}
	}
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

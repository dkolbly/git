package git

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"
)

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
					fmt.Printf("  including pack %s\n", f.Name())
					g.packs = append(g.packs, p)
				} else {
					fmt.Printf("  %s failed: %s\n", f.Name(), err)
				}
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

package git

import (
	"encoding/hex"
	"errors"
	"fmt"
)

type ObjType int

const (
	ObjNone = ObjType(iota)
	ObjCommit
	ObjTree
	ObjBlob
	ObjTag
	objFutureExpansion
	ObjOffsetDelta
	ObjRefDelta
)

var typeFromString = map[string]ObjType{
	"tree":   ObjTree,
	"commit": ObjCommit,
	"blob":   ObjBlob,
	"tag":    ObjTag,
}

func (t ObjType) String() string {
	switch t {
	case ObjCommit:
		return "commit"
	case ObjTree:
		return "tree"
	case ObjBlob:
		return "blob"
	case ObjTag:
		return "tag"
	case objFutureExpansion:
		return "--"
	case ObjOffsetDelta:
		return "offset-delta"
	case ObjRefDelta:
		return "ref-delta"
	default:
		return fmt.Sprintf("ObjType(%d)", t)
	}
}

type GitObject interface {
	Name() *Ptr
	Type() ObjType
	Payload() ([]byte, error)
	Load() (GitObject, error)
}

type Ptr struct {
	hash [20]byte
}

// returns true if q is strictly less than p
func (p *Ptr) Less(q *Ptr) bool {
	for i := 0; i < 20; i++ {
		if q.hash[i] < p.hash[i] {
			return true
		}
		if q.hash[i] != p.hash[i] {
			return false
		}
	}
	return false
}

func (p *Ptr) Equals(q *Ptr) bool {
	for i := 0; i < 20; i++ {
		if p.hash[i] != q.hash[i] {
			return false
		}
	}
	return true
}

func ParsePtr(s string) (p Ptr, ok bool) {
	buf, err := hex.DecodeString(s)
	if err != nil {
		return
	}
	if len(buf) != 20 {
		return
	}
	ok = true
	copy(p.hash[:], buf)
	return
}

func (p *Ptr) String() string {
	return hex.EncodeToString(p.hash[:])
}

func objParse(hexref string) (ret Ptr, ok bool) {
	z, err := hex.DecodeString(hexref)
	if err != nil {
		return
	}
	if len(z) != 20 {
		return
	}
	copy(ret.hash[:], z)
	ok = true
	return
}

func (g *Git) ExpandRef(ref string) (*Ptr, error) {
	z, err := hex.DecodeString(ref)
	if err != nil {
		return nil, err
	}
	if len(z) != 20 {
		return nil, ErrInvalidRef
	}
	p := &Ptr{}
	copy(p.hash[:], z)
	return p, nil
}

var ErrInvalidRef = errors.New("invalid reference")

func (of *ObjFile) Close() error {
	of.unz.Close()
	return of.raw.Close()
}

func (of *ObjFile) Read(buf []byte) (int, error) {
	return of.unz.Read(buf)
}

func (g *Git) verify(n *Ptr, t ObjType, data []byte) bool {
	// TODO: verify hash
	return true
}

func (g *Git) loadInterp(n *Ptr, t ObjType, data []byte) (GitObject, error) {
	switch t {
	case ObjTree:
		return g.loadTree(n, data)
	case ObjCommit:
		return g.loadCommit(n, data)
	default:
		return nil, ErrUnknownObjectType
	}
}

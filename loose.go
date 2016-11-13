package git

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"path"
)

type LooseObject struct {
	repo   *Git
	name   Ptr
	file   string
	loaded GitObject
}

func (l *LooseObject) Type() ObjType {
	x, err := l.Load()
	if err != nil {
		return ObjNone
	}
	return x.Type()
}

func (l *LooseObject) Name() *Ptr {
	return &l.name
}

func (l *LooseObject) Payload() ([]byte, error) {
	rdr, err := os.Open(l.file)
	if err != nil {
		return nil, err
	}
	defer rdr.Close()

	rc, err := zlib.NewReader(rdr)
	if err != nil {
		return nil, err
	}

	defer rc.Close()
	return ioutil.ReadAll(rc)
}

func (l *LooseObject) Load() (GitObject, error) {
	if l.loaded != nil {
		return l.loaded, nil
	}

	buf, err := l.Payload()
	if err != nil {
		return nil, err
	}

	k := bytes.IndexByte(buf, ' ')
	if k < 0 {
		return nil, ErrUnknownObjectType
	}

	z := bytes.IndexByte(buf, 0)
	// TODO double-check the hash (assert SHA1(buf) == l.name)
	// TODO double-check the length

	t := typeFromString[string(buf[:k])]
	return l.repo.loadInterp(&l.name, t, buf[z+1:])
	/*
		switch string(buf[:k]) {
		case "tree":
			l.loaded, err = l.repo.loadTree(&l.name, buf[z+1:])
		default:
			return nil, ErrUnknownObjectType
		}
		return l.loaded, err

	*/
	//return &ObjFile{raw: rdr, unz: rc}, nil
}

/*func (g *Git) newLoose(p *Ptr) *LooseObject {
	return &LooseObject{
		repo: g,
		name: *p,
	}
}*/

func (g *Git) getLoose(p *Ptr) *LooseObject {
	h := hex.EncodeToString(p.hash[:])
	f := path.Join(g.Dir, "objects", h[:2], h[2:])

	_, err := os.Stat(f)
	if err != nil {
		return nil
	}
	return &LooseObject{
		repo: g,
		name: *p,
		file: f,
	}
}

/*func (g *Git) Get(p *Ptr) (io.ReadCloser, error) {
	h := hex.EncodeToString(p.hash[:])
	f := path.Join(g.Dir, "objects", h[:2], h[2:])

	rdr, err := os.Open(f)
	if err != nil {
		return nil, err
	}

	rc, err := zlib.NewReader(rdr)
	if err != nil {
		rdr.Close()
		return nil, err
	}

	return &ObjFile{raw: rdr, unz: rc}, nil
}
*/

type ObjFile struct {
	raw *os.File
	unz io.ReadCloser
}

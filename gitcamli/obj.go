package gitcamli

import (
	"bytes"
	"errors"
	"io/ioutil"

	"camlistore.org/pkg/blob"
	"github.com/dkolbly/git"
)

type CamliGitObject struct {
	name   git.Ptr
	owner  *Client
	cached []byte
}

func (cc *Client) Get(obj *git.Ptr) git.GitObject {
	cg := cc.cache[*obj]
	if cg == nil {
		cg = &CamliGitObject{
			name:  *obj,
			owner: cc,
		}
		cc.cache[*obj] = cg
	}
	return cg
}

func (cg *CamliGitObject) Name() *git.Ptr {
	return &cg.name
}

func (cg *CamliGitObject) contents() ([]byte, error) {
	if cg.cached != nil {
		return cg.cached, nil
	}
	log.Info("getting %s", &cg.name)
	ref, ok := blob.Parse("sha1-" + cg.Name().String())
	if !ok {
		panic("could not turn git ref into blob ref")
	}

	src, n, err := cg.owner.camli.Fetch(ref)
	if err != nil {
		log.Error("%s failed: %s", ref, err)
		return nil, err
	}

	buf, err := ioutil.ReadAll(src)
	if err != nil {
		return nil, err
	}
	if uint32(len(buf)) != n {
		log.Error("Read %d but expected %d", len(buf), n)
		return nil, ErrShortRead
	}
	cg.cached = buf
	return buf, nil
}

var ErrShortRead = errors.New("short read")

var typeFromString = map[string]git.ObjType{
	"tree":   git.ObjTree,
	"commit": git.ObjCommit,
	"blob":   git.ObjBlob,
	"tag":    git.ObjTag,
}

func (cg *CamliGitObject) Type() git.ObjType {
	buf, err := cg.contents()
	if err != nil {
		return git.ObjNone
	}

	k := bytes.IndexByte(buf, ' ')
	return typeFromString[string(buf[:k])]
}

func (cg *CamliGitObject) Payload() ([]byte, error) {
	return cg.contents()
}

func (cg *CamliGitObject) Load() (git.GitObject, error) {
	buf, err := cg.contents()
	if err != nil {
		return nil, err
	}
	k := bytes.IndexByte(buf, 0)
	return cg.owner.repo.Interpret(&cg.name, cg.Type(), buf[k+1:])
}

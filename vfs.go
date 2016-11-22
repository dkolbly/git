package git

import (
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"golang.org/x/tools/godoc/vfs"
)

type gitFS struct {
	root *Tree
}

func (fs *gitFS) String() string {
	return "git(" + fs.root.name.String()[0:7] + ")"
}

var ErrCorrupt = errors.New("corrupt repository")
var ErrNoEntry = errors.New("no such file or directory")
var ErrIsDir = errors.New("is a directory")
var ErrNotDir = errors.New("not a directory")
var ErrNotBlob = errors.New("not a blob")

func (fs *gitFS) Open(name string) (vfs.ReadSeekCloser, error) {
	n := fs.root.Walk(name)
	if n == nil {
		return nil, ErrNoEntry
	}
	if n.IsDir() {
		return nil, ErrIsDir
	}
	// n.Ref should refer to a blob
	g := fs.root.repo
	obj, err := g.Get(&n.Ref).Load()
	if err != nil {
		return nil, err
	}

	if blob, ok := obj.(*Blob); ok {
		return blob.Open(), nil
	}
	return nil, ErrNotBlob
}

type nodeFileInfo struct {
	repo   *Git
	n      *Node
	target GitObject
}

func (nfi *nodeFileInfo) String() string {
	return fmt.Sprintf("{%s (%s %s) %.4x}",
		nfi.Name(),
		nfi.Mode(),
		nfi.ModTime(),
		nfi.n.Ref.hash[:],
	)
}

// Name implements os.FileInfo
func (nfi *nodeFileInfo) Name() string {
	return nfi.n.Name
}

// IsDir implements os.FileInfo
func (nfi *nodeFileInfo) IsDir() bool {
	return nfi.n.IsDir()
}

// Size implements os.FileInfo
func (nfi *nodeFileInfo) Size() int64 {
	if nfi.target == nil {
		o, err := nfi.repo.Get(&nfi.n.Ref).Load()
		if err != nil {
			log.Error("Failed: %s", err)
			return 0
		}
		nfi.target = o
	}
	switch item := nfi.target.(type) {
	case *Blob:
		return int64(len(item.data))
	case *Tree:
		return int64(len(item.contents))
	default:
		log.Info("unknown %#v", item)
		return 0
	}
}

// Mode implements os.FileInfo
func (nfi *nodeFileInfo) Mode() os.FileMode {
	mode := nfi.n.Perm & 0777
	if nfi.n.IsDir() {
		mode |= uint(os.ModeDir)
	}
	if nfi.n.IsSymLink() {
		mode |= uint(os.ModeSymlink)
	}
	return os.FileMode(mode)
}

// ModTime implements os.FileInfo
func (nfi *nodeFileInfo) ModTime() time.Time {
	// TODO, ideally we would return the timestamp of the first
	// commit that introduces this blob (since blobs are immutable,
	// we know it is not modified after that!)
	return time.Now()
}

// Sys implements os.FileInfo
func (nfi *nodeFileInfo) Sys() interface{} {
	return nfi.n
}

func (fs *gitFS) walk(posn string, follow bool) (*Node, error) {
	for {
		n := fs.root.Walk(posn)
		if n == nil {
			return nil, ErrNoEntry
		}
		log.Info("%s perm %o", posn, n.Perm)
		if !n.IsSymLink() || !follow {
			log.Info("IsSymLink=%t follow=%t", n.IsSymLink(), follow)
			return n, nil
		}
		// it's a symbol link and we're in follow mode... keep looking
		x := fs.root.repo.Get(&n.Ref)
		obj, err := x.Load()
		if err != nil {
			return nil, err
		}
		if blob, ok := obj.(*Blob); ok {
			log.Info("Following from %q -> %q", posn, blob.Value())
			posn = path.Join(path.Dir(posn), blob.Value())
			log.Info("current posn %q", posn)
		} else {
			// symlink value is not a blob?
			return nil, ErrCorrupt
		}
	}
}

func (fs *gitFS) stat(path string, follow bool) (os.FileInfo, error) {
	n, err := fs.walk(path, follow)
	if err != nil {
		return nil, err
	}
	return &nodeFileInfo{
		repo: fs.root.repo,
		n:    n,
	}, nil
}

func (fs *gitFS) Lstat(path string) (os.FileInfo, error) {
	return fs.stat(path, false)
}

func (fs *gitFS) Stat(path string) (os.FileInfo, error) {
	return fs.stat(path, true)
}

func (fs *gitFS) dirtree(path string) (*Tree, error) {
	if path == "" || path == "." {
		return fs.root, nil
	}
	n, err := fs.walk(path, true)
	if err != nil {
		return nil, err
	}
	if !n.IsDir() {
		return nil, ErrNotDir
	}

	o := fs.root.repo.Get(&n.Ref)
	if o == nil {
		panic("bad ref")
	}
	o, err = o.Load()
	if err != nil {
		panic(err)
	}
	if t, ok := o.(*Tree); ok {
		return t, nil
	}
	// WTF?  We already know its a directory; this repository
	// is corrupted!
	return nil, ErrCorrupt
}

func (fs *gitFS) ReadDir(path string) ([]os.FileInfo, error) {
	t, err := fs.dirtree(path)
	if err != nil {
		return nil, err
	}

	num := len(t.contents)
	fi := make([]os.FileInfo, 0, num)
	for _, v := range t.contents {
		nfi := &nodeFileInfo{
			repo: fs.root.repo,
			n:    v,
		}
		fi = append(fi, nfi)
	}
	return fi, nil
}

func (t *Tree) VFS() vfs.FileSystem {
	return &gitFS{t}
}

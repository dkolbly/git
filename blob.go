package git

import (
	"bytes"
)

type Blob struct {
	repo *Git
	name Ptr
	data []byte
}

func (g *Git) loadBlob(n *Ptr, data []byte) (*Blob, error) {
	return &Blob{
		repo: g,
		name: *n,
		data: data,
	}, nil
}

func (b *Blob) Value() string {
	return string(b.data)
}

func (b *Blob) Name() *Ptr {
	return &b.name
}

func (b *Blob) Type() ObjType {
	return ObjBlob
}

func (b *Blob) Payload() ([]byte, error) {
	return b.data, nil
}

func (b *Blob) Load() (GitObject, error) {
	return b, nil
}

func (b *Blob) Open() *BlobReader {
	return &BlobReader{bytes.NewReader(b.data)}
}

type BlobReader struct {
	*bytes.Reader
}

func (br *BlobReader) Read(buf []byte) (int, error) {
	return br.Reader.Read(buf)
}

func (br *BlobReader) Seek(off int64, whence int) (int64, error) {
	return br.Reader.Seek(off, whence)
}

func (br *BlobReader) Close() error {
	br.Reader = nil
	return nil
}

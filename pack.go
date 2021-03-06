package git

import (
	"compress/zlib"
	"io"
	"reflect"
	"sort"
	"unsafe"

	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

type PackFile struct {
	repo             *Git
	Pack             string
	Index            string
	firstLevelFanout [256][4]byte // from the index
	indexContents    []Ptr
	indexPtrs        []IndexPtr
	indexCRCs        []uint32
	crossRef         map[uint32]int
	data             *os.File
	cache            map[int64]*PackedObject
}

func (p *PackFile) GetNamed(RefType, string) *NamedRef {
	return nil
}

func (p *PackFile) open() (*os.File, error) {
	if p.data == nil {
		f, err := os.Open(p.Pack)
		if err != nil {
			return nil, err
		}
		p.data = f
	}
	return p.data, nil
}

type PackedObject struct {
	name        Ptr
	container   *PackFile
	offset      int64
	size        int64
	typecode    ObjType
	headerlen   uint8
	dedeltatype ObjType
	dedelta     []byte
}

func (po *PackedObject) Load() (GitObject, error) {
	buf, t, err := po.deDeltaifiedBytes(0)
	if err != nil {
		return nil, err
	}
	return po.container.repo.Interpret(&po.name, t, buf)
}

var ErrUnknownObjectType = errors.New("unknown object type")

func (p *PackFile) newPackedObject(obj *Ptr, at int64) (*PackedObject, error) {

	if p.cache[at] != nil {
		return p.cache[at], nil
	}

	data, err := p.open()
	if err != nil {
		return nil, err
	}

	data.Seek(at, 0)

	var header [10]byte
	n, err := data.Read(header[:])
	if err != nil {
		return nil, err
	}
	if n != 10 {
		return nil, fmt.Errorf("only read %d of header", n)
	}

	var size uint64
	var typeCode byte

	typeCode = (header[0] >> 4) & 7
	size = uint64(header[0] & 0xf)
	i := 0
	shift := uint(4)
	//fmt.Printf("at %d, size=%d\n", i, size)
	for (header[i] & 0x80) != 0 {
		i++
		size += uint64(header[i]&0x7f) << shift
		shift += 7
		//fmt.Printf("at %d, size=%d\n", i, size)
	}

	po := &PackedObject{
		name:        *obj,
		container:   p,
		offset:      at,
		size:        int64(size),
		typecode:    ObjType(typeCode),
		headerlen:   uint8(i + 1),
		dedeltatype: ObjNone,
	}
	p.cache[at] = po
	return po, nil
}

func (po *PackedObject) Name() *Ptr {
	return &po.name
}

func (po *PackedObject) Type() ObjType {
	return po.typecode
}

func decodeOffsetDelta(chunk []byte) (int64, []byte) {
	delta_rel_offset := int64(chunk[0] & 0x7f)
	i := 0
	//fmt.Printf("   offset chunk[0] 0x%02x\n", chunk[0])
	for (chunk[i] & 0x80) != 0 {
		i++
		//fmt.Printf("   offset chunk[%d] 0x%02x\n", i, chunk[i])
		delta_rel_offset = ((1 + delta_rel_offset) << 7) + int64(chunk[i]&0x7f)
	}
	return delta_rel_offset, chunk[i+1:]
}

func (po *PackedObject) Payload() ([]byte, error) {
	//log.Info("Want Payload(%s)", &po.name)
	buf, t, err := po.deDeltaifiedBytes(0)
	if err != nil {
		return nil, err
	}

	// a bit inefficient in the common case; we are copying the payload bytes
	// around... it'd be better to let read() insert the preamble, at least
	// in the no-delta case
	preamble := fmt.Sprintf("%s %d\x00", t, len(buf))
	//log.Info("%s preamble => %s", &po.name, preamble)

	out := make([]byte, len(preamble)+len(buf))
	copy(out[0:len(preamble)], []byte(preamble))
	copy(out[len(preamble):], buf)
	return out, err
}

func (po *PackedObject) deDeltaifiedBytes(depth int) ([]byte, ObjType, error) {

	if po.dedeltatype != ObjNone {
		return po.dedelta, po.dedeltatype, nil
	}

	//log.Info("deDelatify[%d](%s)", depth, &po.name)
	buf, base, err := po.read()
	if base == nil {
		//log.Info("   leaf %s : %d bytes", po.typecode, len(buf))
		po.dedeltatype = po.typecode
		po.dedelta = buf
		return buf, po.typecode, err
	}

	if base.offset == 0 {
		panic("we only handle offset deltas so far")
	}
	p := po.container
	i, ok := p.crossRef[uint32(base.offset)]
	if !ok {
		panic("not a real offset")
	}
	baseObj, err := p.newPackedObject(&(p.indexContents[i]), base.offset)
	if err != nil {
		return nil, ObjNone, err
	}
	baseData, t, err := baseObj.deDeltaifiedBytes(depth + 1)
	if err != nil {
		//fmt.Printf("Could not read base object %s!\n", &baseObj.name)
		return nil, ObjNone, err
	}
	data, ptr, err := patchDelta(t, baseData, buf)
	if err != nil {
		return nil, ObjNone, err
	}

	//log.Info("patched into %s (%d bytes)  we want %s", ptr, len(data), &po.name)
	// make sure the hash of the result of applying the delta
	// is what we expect
	if !ptr.Equals(&po.name) {
		return nil, ObjNone, ErrDeltaMismatch
	}
	po.dedeltatype = t
	po.dedelta = data
	return data, t, err
}

var ErrDeltaMismatch = errors.New("expanded delta name mismatch")

type BaseSpec struct {
	name   *Ptr
	offset int64
}

func (po *PackedObject) read() ([]byte, *BaseSpec, error) {
	var base *BaseSpec

	data := po.container.data
	data.Seek(po.offset+int64(po.headerlen), 0)
	/*log.Info("<%s>\noffset = %d  headerlen = %d  size = %d  type=%s",
	&po.name,
	po.offset,
	po.headerlen,
	po.size,
	po.typecode)*/
	var chunk [10]byte
	n, err := data.Read(chunk[:])
	h := chunk[:n]

	if po.typecode == ObjOffsetDelta {
		delta_rel_offset, h2 := decodeOffsetDelta(h)
		/*fmt.Printf("offset delta %d   ; implies offset @%d\n",
		delta_rel_offset,
		po.offset-delta_rel_offset)*/
		h = h2
		base = &BaseSpec{
			offset: po.offset - delta_rel_offset,
		}
	}

	data.Seek(po.offset+int64(po.headerlen)+10-int64(len(h)), 0)
	rc, err := zlib.NewReader(data)
	if err != nil {
		panic(err)
	}
	defer rc.Close()

	//fmt.Printf("Trying to read %d bytes\n", po.size)
	buf := make([]byte, po.size)
	num, err := io.ReadFull(rc, buf)
	//fmt.Printf("Read %d with error: %s\n", num, err)
	if err != nil {
		if err != io.EOF || num != len(buf) {
			return nil, base, err
		}
	}
	return buf[:num], base, nil
}

func (p *PackFile) Get(obj *Ptr) GitObject {
	at := p.find(obj)
	if at == 0 {
		return nil
	}
	item, err := p.newPackedObject(obj, at)
	if err != nil {
		return nil
	}
	return item
}

// returns the offset of the object in this packfile, or 0 if not present
func (p *PackFile) find(obj *Ptr) int64 {
	i := obj.hash[0]
	var a, b int
	if i > 0 {
		a = int(binary.BigEndian.Uint32(p.firstLevelFanout[i-1][:]))
	}
	b = int(binary.BigEndian.Uint32(p.firstLevelFanout[i][:]))
	if a > b {
		panic("bad pack index")
	}
	if a == b {
		// empty region
		return 0
	}
	if a+1 == b {
		// only one thing in region
		if obj.Equals(&p.indexContents[a]) {
			return (&p.indexPtrs[a]).asOffset()
		}
	}
	k := sort.Search(b-a, func(i int) bool {
		return !obj.Less(&p.indexContents[a+i])
	})
	if obj.Equals(&p.indexContents[a+k]) {
		return (&p.indexPtrs[a+k]).asOffset()
	}
	return 0
}

type IndexPtr [4]byte

func (ip *IndexPtr) asOffset() int64 {
	u := binary.BigEndian.Uint32((*ip)[:])
	return int64(u)
}

type memBlock interface{}

func readRaw(dest memBlock, nbytes int, src *os.File) (int, error) {
	var raw []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&raw))

	dv := reflect.ValueOf(dest)
	if dv.Kind() == reflect.Ptr {
		hdr.Data = dv.Elem().UnsafeAddr()
	} else if dv.Kind() == reflect.Slice {
		hdr.Data = dv.Pointer()
	}
	hdr.Len = nbytes
	hdr.Cap = nbytes
	/*at, _ := src.Seek(0, 1)
	fmt.Printf("Reading %d bytes at +%d...\n", nbytes, at)*/
	return src.Read(raw)
}

func (p *PackFile) loadIndex() error {
	rdr, err := os.Open(p.Index)
	if err != nil {
		return err
	}
	defer rdr.Close()

	var version, signature uint32
	binary.Read(rdr, binary.BigEndian, &signature)
	binary.Read(rdr, binary.BigEndian, &version)

	readRaw(&p.firstLevelFanout, 256*4, rdr)
	/*
		for i, f := range p.firstLevelFanout[:] {
			fmt.Printf("  [0x%02x] %#x %d\n", i, f[:], binary.BigEndian.Uint32(f[:]))
		}*/

	count := int(binary.BigEndian.Uint32(p.firstLevelFanout[255][:]))
	entries := make([]Ptr, count)
	crctable := make([]uint32, count)
	ptrs := make([]IndexPtr, count)
	crossRef := make(map[uint32]int, count)

	readRaw(entries, count*20, rdr)
	readRaw(crctable, count*4, rdr)
	readRaw(ptrs, count*4, rdr)

	for i := 0; i < count; i++ {
		offset := binary.BigEndian.Uint32(ptrs[i][:])
		crossRef[offset] = i
	}

	/*	for i := 0; i < count; i++ {
		offset := binary.BigEndian.Uint32(ptrs[i][:])
		fmt.Printf("  [%d] %s  @%d\n", i, &entries[i], offset)
	}*/
	p.indexContents = entries
	p.indexCRCs = crctable
	p.indexPtrs = ptrs
	p.crossRef = crossRef
	return nil
}

func IncludePackFile(g *Git, pack string) (*PackFile, error) {
	p := &PackFile{
		repo:  g,
		Pack:  pack,
		Index: pack[:len(pack)-5] + ".idx",
		cache: make(map[int64]*PackedObject),
	}
	err := p.loadIndex()
	if err != nil {
		return nil, err
	}
	log.Info("Including %s with %d items", pack, len(p.indexContents))
	g.AddStore(p)
	return p, nil
}

/*
	rdr, err := os.Open(f)
	if err != nil {
		return err
	}

	var signature, version, count uint32
	binary.Read(rdr, binary.BigEndian, &signature)
	binary.Read(rdr, binary.BigEndian, &version)
	binary.Read(rdr, binary.BigEndian, &count)

	if signature != GitPackSignature {
		return ErrNotAPack
	}

	fmt.Printf("signature = %#x\n", signature)
	fmt.Printf("version = %d\n", version)
	fmt.Printf("count = %d\n", count)

	for i :=uint32(0); i<count; i++ {
		var p Ptr
		rdr.Read(p.hash[:])
		fmt.Printf("  [%d] %s\n", i, &p)
	}
	return nil
}
*/

/*
 * The per-object header is a pretty dense thing, which is
 *  - first byte: low four bits are "size", then three bits of "type",
 *    and the high bit is "size continues".
 *  - each byte afterwards: low seven bits are size continuation,
 *    with the high bit being "size continues"
 */

var ErrNotAPack = errors.New("not a pack file")

const GitPackSignature = 0x5041434b

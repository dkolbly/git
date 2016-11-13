package git

import (
	"crypto/sha1"
	"errors"
	"fmt"
)

func deltaHdrSize(src []byte) (int, []byte) {
	// pop quiz: how many variable-length size encodings are there in git?

	var size int
	shift := uint(0)
	for {
		cmd := src[0]
		src = src[1:]
		size = size | (int(cmd&0x7f) << shift)
		shift += 7
		if cmd&0x80 == 0 || len(src) == 0 {
			return size, src
		}
	}
}

var ErrBadDelta = errors.New("corrupt delta")

func patchDelta(mode ObjType, base, delta []byte) ([]byte, *Ptr, error) {
	baseSize, delta := deltaHdrSize(delta)
	if baseSize != len(base) {
		return nil, nil, ErrBadDelta
	}

	resultSize, delta := deltaHdrSize(delta)

	result := make([]byte, resultSize)

	check := sha1.New()

	fmt.Printf("    %d byte base, %d byte result, type %s\n", baseSize, resultSize, mode)

	fmt.Fprintf(check, "%s %d", mode, resultSize)
	check.Write([]byte{0})

	j := 0
	i := 0
	remain := resultSize
	for i < len(delta) {
		cmd := delta[i]
		i++

		if cmd&0x80 != 0 {
			// "copy from base"
			// an interesting and unique length encoding; it
			// supports (don't know if they use) interior and right
			// 00 compression!
			var copyOffset, copySize int

			if (cmd & 0x01) != 0 {
				copyOffset = int(delta[i])
				i++
			}
			if (cmd & 0x02) != 0 {
				copyOffset |= int(delta[i]) << 8
				i++
			}
			if (cmd & 0x04) != 0 {
				copyOffset |= int(delta[i]) << 16
				i++
			}
			if (cmd & 0x08) != 0 {
				copyOffset |= int(delta[i]) << 24
				i++
			}
			if (cmd & 0x10) != 0 {
				copySize = int(delta[i])
				i++
			}
			if (cmd & 0x20) != 0 {
				copySize |= int(delta[i]) << 8
				i++
			}
			if (cmd & 0x40) != 0 {
				copySize |= int(delta[i]) << 16
				i++
			}
			if copySize == 0 {
				// default to a 64K chunk
				copySize = 0x10000
			}
			fmt.Printf("    copy from base: %d bytes from offset %d\n%x\n",
				copySize, copyOffset, base[copyOffset:copyOffset+copySize])
			copy(result[j:j+copySize], base[copyOffset:])
			check.Write(base[copyOffset : copyOffset+copySize])

			j += copySize
			remain -= copySize

		} else if cmd != 0 {
			// copy from delta, up to 127 bytes
			copySize := int(cmd)
			if copySize > remain {
				break
			}
			copy(result[j:j+copySize], delta[i:])
			check.Write(delta[i : i+copySize])
			fmt.Printf("    copy from delta: %d bytes from offset %d\n%x\n",
				cmd, i, delta[i:i+copySize])

			i += copySize
			j += copySize
			remain -= copySize
		} else {
			return nil, nil, ErrUnexpectedDeltaOpcode
		}
	}
	if j != resultSize {
		fmt.Printf("didn't seem to write it all??\n")
	}
	var p Ptr
	copy(p.hash[:], check.Sum(nil))
	return result, &p, nil
}

var ErrUnexpectedDeltaOpcode = errors.New("unexpected delta opcode 0")

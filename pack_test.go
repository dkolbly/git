package git

import (
	"testing"
)

type relativeDeltaEncodingCase struct {
	expect   int64
	encoding []byte
}

func TestRelativeDeltaDecoding(t *testing.T) {
	cases := []relativeDeltaEncodingCase{
		{13334, []byte{0xe7, 0x16}},
		{160, []byte{0x80, 0x20}},
		{26777, []byte{0x80, 0xd0, 0x19}},
	}
	for _, c := range cases {
		delta, remain := decodeOffsetDelta(c.encoding)
		if len(remain) != 0 {
			t.Fatalf("Expected to consume entire thing, left %d bytes", len(remain))
		}
		if delta != c.expect {
			t.Fatalf("Expected delta %d, got %d", c.expect, delta)
		}
	}
}

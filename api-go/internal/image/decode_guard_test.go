package image

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"strings"
	"testing"

	"openposterdb/internal/services"
)

// pngHeaderOnly builds a PNG whose IHDR declares w x h but carries no pixel
// data — DecodeConfig accepts it, a full Decode would allocate w*h*4 bytes.
func pngHeaderOnly(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	chunk := append([]byte("IHDR"), ihdr...)
	binary.Write(&buf, binary.BigEndian, uint32(len(ihdr)))
	buf.Write(chunk)
	binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return buf.Bytes()
}

func TestDecodeImage_RejectsOversizedDimensions(t *testing.T) {
	_, err := decodeImage(pngHeaderOnly(50000, 50000))
	if err == nil || !strings.Contains(err.Error(), "pixel limit") {
		t.Fatalf("want pixel-limit error, got %v", err)
	}
	if _, err := decodeImage(SamplePosterPNG); err != nil {
		t.Fatalf("normal poster must decode: %v", err)
	}
}

func TestReadAllLimited(t *testing.T) {
	if b, err := services.ReadAllLimited(strings.NewReader("abcd"), 4); err != nil || string(b) != "abcd" {
		t.Fatalf("at limit: %q %v", b, err)
	}
	if _, err := services.ReadAllLimited(strings.NewReader("abcde"), 4); err == nil {
		t.Fatal("over limit must error, not truncate")
	}
}

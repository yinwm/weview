//go:build cgo

package media

import (
	"encoding/binary"
	"testing"
)

func TestEncodePCM16MonoWAVHeader(t *testing.T) {
	pcm := []byte{0x01, 0x00, 0xff, 0x7f}
	wav := encodePCM16MonoWAV(pcm, 24000)
	if len(wav) != 44+len(pcm) {
		t.Fatalf("wav length = %d, want %d", len(wav), 44+len(pcm))
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || string(wav[36:40]) != "data" {
		t.Fatalf("invalid wav header: %q %q %q", wav[0:4], wav[8:12], wav[36:40])
	}
	if got := binary.LittleEndian.Uint32(wav[24:28]); got != 24000 {
		t.Fatalf("sample rate = %d, want 24000", got)
	}
	if got := binary.LittleEndian.Uint16(wav[34:36]); got != 16 {
		t.Fatalf("bits per sample = %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint32(wav[40:44]); got != uint32(len(pcm)) {
		t.Fatalf("data size = %d, want %d", got, len(pcm))
	}
}

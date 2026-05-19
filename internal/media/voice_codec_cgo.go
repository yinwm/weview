//go:build cgo

package media

import (
	"encoding/binary"
	"fmt"

	silkcodec "github.com/sjzar/go-silk"
)

const voiceSampleRate = 24000

func silkToWAV(data []byte) ([]byte, error) {
	pcm, sampleRate, err := silkToPCM16(data)
	if err != nil {
		return nil, err
	}
	return encodePCM16MonoWAV(pcm, sampleRate), nil
}

func silkToPCM16(data []byte) ([]byte, int, error) {
	payload, err := normalizeSilkPayload(data)
	if err != nil {
		return nil, 0, err
	}
	decoder := silkcodec.SilkInit()
	defer decoder.Close()
	decoder.SetSampleRate(voiceSampleRate)
	pcm := decoder.Decode(payload)
	pcm = append(pcm, decoder.Flush()...)
	if len(pcm) == 0 {
		return nil, 0, fmt.Errorf("silk decode failed")
	}
	if len(pcm)%2 != 0 {
		return nil, 0, fmt.Errorf("invalid pcm length: %d", len(pcm))
	}
	return pcm, voiceSampleRate, nil
}

func encodePCM16MonoWAV(pcm []byte, sampleRate int) []byte {
	const (
		channels      = 1
		bitsPerSample = 16
		headerSize    = 44
	)
	out := make([]byte, headerSize+len(pcm))
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+len(pcm)))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], channels)
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(sampleRate*channels*bitsPerSample/8))
	binary.LittleEndian.PutUint16(out[32:34], channels*bitsPerSample/8)
	binary.LittleEndian.PutUint16(out[34:36], bitsPerSample)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(len(pcm)))
	copy(out[44:], pcm)
	return out
}

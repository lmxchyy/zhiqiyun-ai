package media

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// ConcatenatePCM16MonoWAVs merges PCM16 mono WAV clips that share the same sample rate.
func ConcatenatePCM16MonoWAVs(clips [][]byte) ([]byte, error) {
	if len(clips) == 0 {
		return nil, fmt.Errorf("no wav clips to concatenate")
	}
	var sampleRate uint32
	var pcm bytes.Buffer
	for i, clip := range clips {
		rate, data, err := decodePCM16MonoWAV(clip)
		if err != nil {
			return nil, fmt.Errorf("clip %d: %w", i, err)
		}
		if i == 0 {
			sampleRate = rate
		} else if rate != sampleRate {
			return nil, fmt.Errorf("clip %d sample rate %d != %d", i, rate, sampleRate)
		}
		pcm.Write(data)
	}
	return encodePCM16MonoWAV(pcm.Bytes(), sampleRate), nil
}

func encodePCM16MonoWAV(pcm []byte, sampleRate uint32) []byte {
	if sampleRate == 0 {
		sampleRate = 24000
	}
	dataSize := uint32(len(pcm))
	buf := make([]byte, 44+len(pcm))
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], 36+dataSize)
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(buf[22:24], 1) // mono
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], sampleRate*2)
	binary.LittleEndian.PutUint16(buf[32:34], 2)
	binary.LittleEndian.PutUint16(buf[34:36], 16)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], dataSize)
	copy(buf[44:], pcm)
	return buf
}

func decodePCM16MonoWAV(raw []byte) (sampleRate uint32, pcm []byte, err error) {
	if len(raw) < 44 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return 0, nil, fmt.Errorf("invalid wav header")
	}
	offset := 12
	var dataChunk []byte
	for offset+8 <= len(raw) {
		chunkID := string(raw[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		offset += 8
		if offset+chunkSize > len(raw) {
			return 0, nil, fmt.Errorf("truncated wav chunk %s", chunkID)
		}
		payload := raw[offset : offset+chunkSize]
		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return 0, nil, fmt.Errorf("invalid fmt chunk")
			}
			audioFormat := binary.LittleEndian.Uint16(payload[0:2])
			channels := binary.LittleEndian.Uint16(payload[2:4])
			sampleRate = binary.LittleEndian.Uint32(payload[4:8])
			bits := binary.LittleEndian.Uint16(payload[14:16])
			if audioFormat != 1 || channels != 1 || bits != 16 {
				return 0, nil, fmt.Errorf("only PCM16 mono wav is supported")
			}
		case "data":
			dataChunk = payload
		}
		offset += chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}
	if sampleRate == 0 || dataChunk == nil {
		return 0, nil, fmt.Errorf("wav missing fmt/data")
	}
	return sampleRate, dataChunk, nil
}

// SilencePCM16MonoWAV creates a silent wav clip of the given duration.
func SilencePCM16MonoWAV(durationMs int64, sampleRate uint32) []byte {
	if sampleRate == 0 {
		sampleRate = 24000
	}
	if durationMs < 0 {
		durationMs = 0
	}
	samples := int(durationMs) * int(sampleRate) / 1000
	return encodePCM16MonoWAV(make([]byte, samples*2), sampleRate)
}

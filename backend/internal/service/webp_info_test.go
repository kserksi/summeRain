// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectWebPExtended(t *testing.T) {
	data := make([]byte, 30)
	copy(data[:4], "RIFF")
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8X")
	binary.LittleEndian.PutUint32(data[16:20], 10)
	// Stored dimensions are value - 1: 400 x 300.
	data[24], data[25] = 0x8f, 0x01
	data[27], data[28] = 0x2b, 0x01

	info, err := inspectWebP(data)
	if err != nil {
		t.Fatalf("inspectWebP returned error: %v", err)
	}
	if info.Width != 400 || info.Height != 300 || info.Animated {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestInspectWebPRejectsAnimation(t *testing.T) {
	data := make([]byte, 30)
	copy(data[:4], "RIFF")
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8X")
	binary.LittleEndian.PutUint32(data[16:20], 10)
	data[20] = 0x02
	data[24], data[27] = 1, 1

	info, err := inspectWebP(data)
	if err != nil {
		t.Fatalf("inspectWebP returned error: %v", err)
	}
	if !info.Animated {
		t.Fatal("expected animated WebP")
	}
}

func TestInspectWebPRejectsInvalidPayload(t *testing.T) {
	if _, err := inspectWebP([]byte("not an image")); err == nil {
		t.Fatal("expected invalid WebP error")
	}
}

func TestInspectCompleteWebPRejectsTruncatedAndTrailingBodies(t *testing.T) {
	valid := simpleLosslessWebP(400, 300)
	info, err := inspectCompleteWebP(bytes.NewReader(valid), int64(len(valid)))
	if err != nil {
		t.Fatalf("valid complete WebP rejected: %v", err)
	}
	if info.Width != 400 || info.Height != 300 || info.Animated {
		t.Fatalf("unexpected complete WebP info: %+v", info)
	}

	for name, payload := range map[string][]byte{
		"truncated": valid[:len(valid)-1],
		"trailing":  append(append([]byte(nil), valid...), 0),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := inspectCompleteWebP(bytes.NewReader(payload), int64(len(payload))); err == nil {
				t.Fatal("malformed complete WebP was accepted")
			}
		})
	}
}

func TestInspectWebPFileValidatesCompleteContainer(t *testing.T) {
	payload := append(simpleLosslessWebP(32, 24), 0)
	path := filepath.Join(t.TempDir(), "trailing.webp")
	if err := os.WriteFile(path, payload, 0640); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := inspectWebPFile(path); err == nil {
		t.Fatal("publish file with trailing data was accepted")
	}
}

func simpleLosslessWebP(width, height int) []byte {
	const chunkSize = 5
	payload := make([]byte, 12+8+chunkSize+1)
	copy(payload[:4], "RIFF")
	binary.LittleEndian.PutUint32(payload[4:8], uint32(len(payload)-8))
	copy(payload[8:12], "WEBP")
	copy(payload[12:16], "VP8L")
	binary.LittleEndian.PutUint32(payload[16:20], chunkSize)
	payload[20] = 0x2f
	bits := uint32(width-1) | uint32(height-1)<<14
	binary.LittleEndian.PutUint32(payload[21:25], bits)
	return payload
}

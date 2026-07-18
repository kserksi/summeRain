// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var errInvalidWebP = errors.New("invalid WebP payload")

type webPInfo struct {
	Width    int
	Height   int
	Animated bool
}

// inspectWebP only needs the beginning of a WebP file. It validates the RIFF
// container and extracts dimensions without decoding pixels into server memory.
func inspectWebP(data []byte) (webPInfo, error) {
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return webPInfo{}, errInvalidWebP
	}

	var info webPInfo
	for offset := 12; offset+8 <= len(data); {
		kind := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		payload := offset + 8
		if size < 0 || payload > len(data) {
			return webPInfo{}, errInvalidWebP
		}

		switch kind {
		case "VP8X":
			if size < 10 || payload+10 > len(data) {
				return webPInfo{}, errInvalidWebP
			}
			info.Animated = data[payload]&0x02 != 0
			info.Width = readUint24(data[payload+4:payload+7]) + 1
			info.Height = readUint24(data[payload+7:payload+10]) + 1
		case "VP8 ":
			if size < 10 || payload+10 > len(data) ||
				data[payload+3] != 0x9d || data[payload+4] != 0x01 || data[payload+5] != 0x2a {
				return webPInfo{}, errInvalidWebP
			}
			info.Width = int(binary.LittleEndian.Uint16(data[payload+6:payload+8]) & 0x3fff)
			info.Height = int(binary.LittleEndian.Uint16(data[payload+8:payload+10]) & 0x3fff)
		case "VP8L":
			if size < 5 || payload+5 > len(data) || data[payload] != 0x2f {
				return webPInfo{}, errInvalidWebP
			}
			bits := binary.LittleEndian.Uint32(data[payload+1 : payload+5])
			info.Width = int(bits&0x3fff) + 1
			info.Height = int((bits>>14)&0x3fff) + 1
		case "ANIM", "ANMF":
			info.Animated = true
		}

		if info.Width > 0 && info.Height > 0 && (kind == "VP8 " || kind == "VP8L" || kind == "VP8X") {
			if info.Animated {
				return info, nil
			}
			// Simple VP8/VP8L files have no animation chunks after the image.
			if kind != "VP8X" {
				return info, nil
			}
		}

		next := payload + size
		if size&1 == 1 {
			next++
		}
		if next <= offset {
			return webPInfo{}, errInvalidWebP
		}
		offset = next
	}

	if info.Width <= 0 || info.Height <= 0 {
		return webPInfo{}, fmt.Errorf("%w: dimensions not found", errInvalidWebP)
	}
	return info, nil
}

// inspectCompleteWebP validates the complete RIFF container while reading only
// small chunk headers into memory. The caller supplies the bytes actually
// written so a truncated body or a forged RIFF length cannot pass validation.
func inspectCompleteWebP(reader io.Reader, actualSize int64) (webPInfo, error) {
	if actualSize < 20 {
		return webPInfo{}, errInvalidWebP
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(reader, header); err != nil {
		return webPInfo{}, fmt.Errorf("%w: read RIFF header: %v", errInvalidWebP, err)
	}
	if string(header[:4]) != "RIFF" || string(header[8:12]) != "WEBP" {
		return webPInfo{}, errInvalidWebP
	}
	declaredSize := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
	if declaredSize != actualSize {
		return webPInfo{}, fmt.Errorf("%w: RIFF size %d does not match body size %d", errInvalidWebP, declaredSize, actualSize)
	}

	remaining := actualSize - 12
	chunkIndex := 0
	extended := false
	bitstreamSeen := false
	encodedWidth, encodedHeight := 0, 0
	var info webPInfo

	for remaining > 0 {
		if remaining < 8 {
			return webPInfo{}, fmt.Errorf("%w: incomplete chunk header", errInvalidWebP)
		}
		var chunkHeader [8]byte
		if _, err := io.ReadFull(reader, chunkHeader[:]); err != nil {
			return webPInfo{}, fmt.Errorf("%w: read chunk header: %v", errInvalidWebP, err)
		}
		remaining -= 8
		kind := string(chunkHeader[:4])
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:]))
		paddedSize := chunkSize + chunkSize&1
		if paddedSize > remaining {
			return webPInfo{}, fmt.Errorf("%w: chunk %s exceeds RIFF boundary", errInvalidWebP, kind)
		}

		prefixSize := int64(0)
		switch kind {
		case "VP8X":
			if chunkIndex != 0 || extended || chunkSize != 10 {
				return webPInfo{}, fmt.Errorf("%w: invalid VP8X chunk", errInvalidWebP)
			}
			var payload [10]byte
			if _, err := io.ReadFull(reader, payload[:]); err != nil {
				return webPInfo{}, fmt.Errorf("%w: read VP8X chunk: %v", errInvalidWebP, err)
			}
			prefixSize = int64(len(payload))
			if payload[0]&0xc1 != 0 || payload[1] != 0 || payload[2] != 0 || payload[3] != 0 {
				return webPInfo{}, fmt.Errorf("%w: VP8X reserved bits are set", errInvalidWebP)
			}
			extended = true
			info.Animated = payload[0]&0x02 != 0
			info.Width = readUint24(payload[4:7]) + 1
			info.Height = readUint24(payload[7:10]) + 1
		case "VP8 ":
			if bitstreamSeen || (!extended && chunkIndex != 0) || chunkSize < 10 {
				return webPInfo{}, fmt.Errorf("%w: invalid VP8 chunk", errInvalidWebP)
			}
			var payload [10]byte
			if _, err := io.ReadFull(reader, payload[:]); err != nil {
				return webPInfo{}, fmt.Errorf("%w: read VP8 chunk: %v", errInvalidWebP, err)
			}
			prefixSize = int64(len(payload))
			if payload[3] != 0x9d || payload[4] != 0x01 || payload[5] != 0x2a {
				return webPInfo{}, fmt.Errorf("%w: invalid VP8 frame header", errInvalidWebP)
			}
			encodedWidth = int(binary.LittleEndian.Uint16(payload[6:8]) & 0x3fff)
			encodedHeight = int(binary.LittleEndian.Uint16(payload[8:10]) & 0x3fff)
			bitstreamSeen = encodedWidth > 0 && encodedHeight > 0
		case "VP8L":
			if bitstreamSeen || (!extended && chunkIndex != 0) || chunkSize < 5 {
				return webPInfo{}, fmt.Errorf("%w: invalid VP8L chunk", errInvalidWebP)
			}
			var payload [5]byte
			if _, err := io.ReadFull(reader, payload[:]); err != nil {
				return webPInfo{}, fmt.Errorf("%w: read VP8L chunk: %v", errInvalidWebP, err)
			}
			prefixSize = int64(len(payload))
			if payload[0] != 0x2f {
				return webPInfo{}, fmt.Errorf("%w: invalid VP8L signature", errInvalidWebP)
			}
			bits := binary.LittleEndian.Uint32(payload[1:])
			encodedWidth = int(bits&0x3fff) + 1
			encodedHeight = int((bits>>14)&0x3fff) + 1
			bitstreamSeen = true
		case "ANIM", "ANMF":
			info.Animated = true
		}

		if chunkSize > prefixSize {
			if _, err := io.CopyN(io.Discard, reader, chunkSize-prefixSize); err != nil {
				return webPInfo{}, fmt.Errorf("%w: read %s payload: %v", errInvalidWebP, kind, err)
			}
		}
		if chunkSize&1 == 1 {
			var padding [1]byte
			if _, err := io.ReadFull(reader, padding[:]); err != nil {
				return webPInfo{}, fmt.Errorf("%w: missing chunk padding", errInvalidWebP)
			}
		}
		remaining -= paddedSize
		chunkIndex++
	}

	if info.Animated {
		if !extended || info.Width <= 0 || info.Height <= 0 {
			return webPInfo{}, fmt.Errorf("%w: animation lacks a VP8X canvas", errInvalidWebP)
		}
		return info, nil
	}
	if !bitstreamSeen {
		return webPInfo{}, fmt.Errorf("%w: image bitstream not found", errInvalidWebP)
	}
	if !extended {
		if chunkIndex != 1 {
			return webPInfo{}, fmt.Errorf("%w: simple WebP contains extra chunks", errInvalidWebP)
		}
		info.Width, info.Height = encodedWidth, encodedHeight
	} else if info.Width != encodedWidth || info.Height != encodedHeight {
		return webPInfo{}, fmt.Errorf("%w: VP8X canvas does not match image frame", errInvalidWebP)
	}
	return info, nil
}

func readUint24(data []byte) int {
	return int(data[0]) | int(data[1])<<8 | int(data[2])<<16
}

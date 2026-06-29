// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GenerateWatermarkSVG(text, color, size string) string {
	if size == "" {
		size = "64"
	}
	if color == "" {
		color = "ffffff"
	}
	width := int(float64(len(text)) * parseFloatDefault(size, 64) * 0.6)
	if width < 10 {
		width = 10
	}
	height := int(parseFloatDefault(size, 64) * 1.25)
	if height < 10 {
		height = 10
	}
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">`+
			`<text x="50%%" y="50%%" text-anchor="middle" dominant-baseline="central" `+
			`font-family="sans-serif" font-size="%s" fill="#%s">%s</text></svg>`,
		width, height, size, color, escapeXML(text),
	)
}

func SaveWatermarkFile(svg, storageBasePath string) error {
	return os.WriteFile(filepath.Join(storageBasePath, "watermark.svg"), []byte(svg), 0644)
}

func RegenerateWatermark(cfgMap map[string]string, storageBasePath string) (changed bool, err error) {
	if cfgMap["watermark_enabled"] != "true" {
		return false, nil
	}
	text := strings.TrimSpace(cfgMap["watermark_text"])
	if text == "" {
		return false, nil
	}
	svg := GenerateWatermarkSVG(text, cfgMap["watermark_color"], cfgMap["watermark_size"])

	path := filepath.Join(storageBasePath, "watermark.svg")
	if existing, err := os.ReadFile(path); err == nil && string(existing) == svg {
		return false, nil
	}
	return true, SaveWatermarkFile(svg, storageBasePath)
}

func escapeXML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

func parseFloatDefault(s string, def float64) float64 {
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil || f <= 0 {
		return def
	}
	return f
}

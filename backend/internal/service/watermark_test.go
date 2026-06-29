// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateWatermarkSVG(t *testing.T) {
	svg := GenerateWatermarkSVG("kserks", "66ccff", "64")
	if !strings.Contains(svg, "<svg") {
		t.Fatal("missing <svg tag")
	}
	if !strings.Contains(svg, `fill="#66ccff"`) {
		t.Fatal("missing fill color")
	}
	if !strings.Contains(svg, `font-size="64"`) {
		t.Fatal("missing font-size")
	}
	if !strings.Contains(svg, ">kserks<") {
		t.Fatal("missing text content")
	}
}

func TestGenerateWatermarkSVGXMLEscape(t *testing.T) {
	svg := GenerateWatermarkSVG(`<script>alert(1)</script>`, "fff", "32")
	if strings.Contains(svg, "<script>") {
		t.Fatal("XML injection: raw <script> tag found in SVG")
	}
	if !strings.Contains(svg, "&lt;script&gt;") {
		t.Fatal("text not XML-escaped")
	}
}

func TestGenerateWatermarkSVGDefaults(t *testing.T) {
	svg := GenerateWatermarkSVG("test", "", "")
	if !strings.Contains(svg, `fill="#ffffff"`) {
		t.Fatal("default color should be ffffff")
	}
	if !strings.Contains(svg, `font-size="64"`) {
		t.Fatal("default size should be 64")
	}
}

func TestRegenerateWatermarkDisabled(t *testing.T) {
	tmp := t.TempDir()
	changed, err := RegenerateWatermark(map[string]string{"watermark_enabled": "false"}, tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("should not change when disabled")
	}
	if _, err := os.Stat(filepath.Join(tmp, "watermark.svg")); !os.IsNotExist(err) {
		t.Fatal("file should not exist when disabled")
	}
}

func TestRegenerateWatermarkCreatesFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := map[string]string{
		"watermark_enabled": "true",
		"watermark_text":    "hello",
		"watermark_color":   "ff0000",
		"watermark_size":    "32",
	}
	changed, err := RegenerateWatermark(cfg, tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("should report changed on first generation")
	}
	data, err := os.ReadFile(filepath.Join(tmp, "watermark.svg"))
	if err != nil {
		t.Fatal("file should exist")
	}
	if !strings.Contains(string(data), "hello") {
		t.Fatal("SVG should contain watermark text")
	}
}

func TestRegenerateWatermarkNoChange(t *testing.T) {
	tmp := t.TempDir()
	cfg := map[string]string{
		"watermark_enabled": "true",
		"watermark_text":    "hello",
		"watermark_size":    "32",
	}
	if _, err := RegenerateWatermark(cfg, tmp); err != nil {
		t.Fatal(err)
	}
	changed, err := RegenerateWatermark(cfg, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("should not report changed when content is identical")
	}
}

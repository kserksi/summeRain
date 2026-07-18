// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
)

func TestWriteOriginalZipEntryStreamsInput(t *testing.T) {
	reader := &trackingArchiveReader{remaining: 8 << 20}
	destination := &countingArchiveWriter{}
	zw := zip.NewWriter(destination)

	if err := writeOriginalZipEntry(zw, "large.webp", reader); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if !reader.closed {
		t.Fatal("source reader was not closed")
	}
	if reader.readCalls < 2 {
		t.Fatalf("source was read in %d call(s), want streaming reads", reader.readCalls)
	}
	if reader.maxRead > 64<<10 {
		t.Fatalf("largest source read was %d bytes, want bounded streaming reads", reader.maxRead)
	}
	if destination.written == 0 {
		t.Fatal("archive writer received no data")
	}
}

func TestWriteOriginalZipEntryUsesFilenameBase(t *testing.T) {
	var destination bytes.Buffer
	zw := zip.NewWriter(&destination)
	if err := writeOriginalZipEntry(zw, `..\..\photo.webp`, io.NopCloser(bytes.NewBufferString("image"))); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(destination.Bytes()), int64(destination.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "photo.webp" {
		t.Fatalf("archive entries = %#v, want one safe basename", zr.File)
	}
	if zr.File[0].Method != zip.Store {
		t.Fatalf("archive method = %d, want store for already-compressed images", zr.File[0].Method)
	}
}

func TestWriteOriginalZipEntryPropagatesReadFailureAndCloses(t *testing.T) {
	wantErr := errors.New("read failed")
	reader := &trackingArchiveReader{remaining: 1, readErr: wantErr}
	zw := zip.NewWriter(io.Discard)

	err := writeOriginalZipEntry(zw, "broken.webp", reader)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if !reader.closed {
		t.Fatal("source reader was not closed after failure")
	}
}

func TestOpenOriginalForArchiveRejectsInvalidStorageLineage(t *testing.T) {
	svc := &ImageService{storageCfg: &config.StorageConfig{BasePath: t.TempDir()}}
	for _, imageFile := range []model.ImageFile{
		{OriginalPath: "original/test.webp", RemoteEndpoint: "https://r2.example"},
		{OriginalPath: "original/test.webp", RemoteBackend: "r2"},
		{OriginalPath: "original/test.webp", RemoteBackend: "s3", RemoteEndpoint: "https://s3.example", RemoteBucket: "images"},
	} {
		if reader, err := svc.openOriginalForArchive(context.Background(), 1, imageFile); err == nil {
			_ = reader.Close()
			t.Fatalf("invalid storage lineage was accepted: %#v", imageFile)
		}
	}
}

func TestOpenOriginalForArchiveRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.webp")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	originalDir := filepath.Join(root, "original")
	if err := os.MkdirAll(originalDir, 0750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(originalDir, "escaped.webp")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	svc := &ImageService{storageCfg: &config.StorageConfig{BasePath: root}}
	if reader, err := svc.openOriginalForArchive(context.Background(), 1, model.ImageFile{OriginalPath: "original/escaped.webp"}); err == nil {
		_ = reader.Close()
		t.Fatal("archive source followed a symlink outside the storage root")
	}
}

type trackingArchiveReader struct {
	remaining int
	maxRead   int
	readCalls int
	readErr   error
	closed    bool
}

func (r *trackingArchiveReader) Read(p []byte) (int, error) {
	r.readCalls++
	if len(p) > r.maxRead {
		r.maxRead = len(p)
	}
	if r.remaining == 0 {
		if r.readErr != nil {
			return 0, r.readErr
		}
		return 0, io.EOF
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for index := 0; index < n; index++ {
		p[index] = byte(index)
	}
	r.remaining -= n
	return n, nil
}

func (r *trackingArchiveReader) Close() error {
	r.closed = true
	return nil
}

type countingArchiveWriter struct {
	written int64
}

func (w *countingArchiveWriter) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	return len(p), nil
}

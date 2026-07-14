// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package imgproxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

type Signer struct {
	key       []byte
	salt      []byte
	publicURL string
}

func NewSigner(hexKey, hexSalt, publicURL string) *Signer {
	key, _ := hex.DecodeString(hexKey)
	salt, _ := hex.DecodeString(hexSalt)
	return &Signer{key: key, salt: salt, publicURL: publicURL}
}

func (s *Signer) Sign(path string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write(s.salt)
	mac.Write([]byte(path))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return s.publicURL + "/" + sig + path
}

func (s *Signer) SignPath(path string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write(s.salt)
	mac.Write([]byte(path))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return "/" + sig + path
}

func (s *Signer) GenerateURLs(sourcePath string, quality int, formats []string) map[string]string {
	urls := make(map[string]string)
	source := "local:///images/" + sourcePath
	for _, f := range formats {
		var path string
		if f == "png" {
			path = fmt.Sprintf("/f:%s/plain/%s", f, source)
		} else {
			path = fmt.Sprintf("/q:%d/f:%s/plain/%s", quality, f, source)
		}
		urls[f] = s.Sign(path)
	}
	return urls
}

func (s *Signer) GenerateThumbnails(sourcePath string, width, height, quality int, formats []string) map[string]string {
	urls := make(map[string]string)
	source := "local:///images/" + sourcePath
	for _, f := range formats {
		var path string
		if f == "png" {
			path = fmt.Sprintf("/rs:fill:%d:%d/f:%s/plain/%s", width, height, f, source)
		} else {
			path = fmt.Sprintf("/rs:fill:%d:%d/q:%d/f:%s/plain/%s", width, height, quality, f, source)
		}
		urls[f] = s.Sign(path)
	}
	return urls
}

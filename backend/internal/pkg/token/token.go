// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

func Generate(byteLen int) (plaintext string, hash string, err error) {
	b := make([]byte, byteLen)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = hex.EncodeToString(b)
	hash = SHA256(plaintext)
	return plaintext, hash, nil
}

func GenerateNonce() (plaintext string, hash string, err error) {
	return Generate(32)
}

func SHA256(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

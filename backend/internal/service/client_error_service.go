// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"log"
	"strings"
	"unicode/utf8"

	"github.com/kserksi/summerain/internal/pkg/errcode"
)

// The client crash-report sink is public and unauthenticated, so its field
// limits are code constants instead of configuration: one report must stay
// small, and a report must never carry credentials.
const (
	clientErrorMessageBytes        = 300
	clientErrorStackBytes          = 2000
	clientErrorComponentStackBytes = 2000
	clientErrorSourceBytes         = 200
	clientErrorPathBytes           = 200
	clientErrorUserAgentBytes      = 200
)

var clientErrorKinds = map[string]struct{}{
	"boundary":           {},
	"error":              {},
	"unhandledrejection": {},
}

// ClientErrorReport is one bounded browser crash report.
type ClientErrorReport struct {
	Kind           string `json:"kind"`
	Message        string `json:"message"`
	Stack          string `json:"stack"`
	ComponentStack string `json:"component_stack"`
	Source         string `json:"source"`
	Path           string `json:"path"`
}

// ClientErrorMeta carries request metadata. Callers must never place cookies,
// tokens, or other credentials into it.
type ClientErrorMeta struct {
	RequestID string
	UserAgent string
}

type ClientErrorService struct{}

func NewClientErrorService() *ClientErrorService {
	return &ClientErrorService{}
}

// Report validates and truncates one crash report, then writes a single
// structured log line. An unsupported kind or an empty message is rejected so
// the sink cannot be reused as a generic log writer.
func (s *ClientErrorService) Report(report ClientErrorReport, meta ClientErrorMeta) *errcode.AppError {
	kind := strings.ToLower(strings.TrimSpace(report.Kind))
	if _, ok := clientErrorKinds[kind]; !ok {
		return errcode.New(3000, "无效的客户端错误类型", 400)
	}
	message := truncateUTF8(strings.TrimSpace(report.Message), clientErrorMessageBytes)
	if message == "" {
		return errcode.New(3000, "客户端错误信息不能为空", 400)
	}

	log.Printf(
		"[client-error] request_id=%s kind=%s path=%q message=%q stack=%q component_stack=%q source=%q user_agent=%q",
		meta.RequestID,
		kind,
		truncateUTF8(strings.TrimSpace(report.Path), clientErrorPathBytes),
		message,
		truncateUTF8(strings.TrimSpace(report.Stack), clientErrorStackBytes),
		truncateUTF8(strings.TrimSpace(report.ComponentStack), clientErrorComponentStackBytes),
		truncateUTF8(strings.TrimSpace(report.Source), clientErrorSourceBytes),
		truncateUTF8(strings.TrimSpace(meta.UserAgent), clientErrorUserAgentBytes),
	)
	return nil
}

// truncateUTF8 keeps the longest prefix that fits in maxBytes without cutting a
// multi-byte rune in half.
func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

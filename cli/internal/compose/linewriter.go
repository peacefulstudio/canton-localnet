// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package compose

import (
	"io"
	"strings"
)

type lineWriter struct {
	passthrough io.Writer
	observe     func(string)
	pending     strings.Builder
}

func newLineWriter(passthrough io.Writer, observe func(string)) *lineWriter {
	return &lineWriter{passthrough: passthrough, observe: observe}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	n, err := w.passthrough.Write(p)
	for _, b := range p[:n] {
		if b != '\n' && b != '\r' {
			w.pending.WriteByte(b)
			continue
		}
		if line := w.pending.String(); line != "" {
			w.observe(line)
		}
		w.pending.Reset()
	}
	return n, err
}

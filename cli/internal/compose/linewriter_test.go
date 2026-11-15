// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package compose

import (
	"bytes"
	"reflect"
	"testing"
)

func TestLineWriterReportsEachCompleteLineAndPassesEveryByteThrough(t *testing.T) {
	var lines []string
	var passedThrough bytes.Buffer
	writer := newLineWriter(&passedThrough, func(line string) { lines = append(lines, line) })

	for _, chunk := range []string{"first\nsec", "ond\r\nthird\rfou", "rth\n", "unfinished"} {
		if _, err := writer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{"first", "second", "third", "fourth"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("want lines %q, got %q", want, lines)
	}
	if got := passedThrough.String(); got != "first\nsecond\r\nthird\rfourth\nunfinished" {
		t.Fatalf("want every byte passed through, got %q", got)
	}
}

func TestLineWriterSkipsBlankLines(t *testing.T) {
	var lines []string
	writer := newLineWriter(&bytes.Buffer{}, func(line string) { lines = append(lines, line) })

	_, _ = writer.Write([]byte("\n\r\nonly\n\n"))

	if !reflect.DeepEqual(lines, []string{"only"}) {
		t.Fatalf("want [only], got %q", lines)
	}
}

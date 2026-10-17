package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHandleArgs(t *testing.T) {
	for _, tt := range []struct {
		args        []string
		done        bool
		code        int
		out, errOut string
	}{
		{nil, false, 0, "", ""},
		{[]string{"--version"}, true, 0, "hax v", ""},
		{[]string{"-h"}, true, 0, "usage: hax", ""},
		{[]string{"--bogus"}, true, 2, "", "unknown argument"},
	} {
		var out, errOut bytes.Buffer
		done, code := handleArgs(tt.args, &out, &errOut)
		if done != tt.done || code != tt.code ||
			!strings.Contains(out.String(), tt.out) ||
			!strings.Contains(errOut.String(), tt.errOut) {
			t.Errorf("%v: done=%v code=%d out=%q err=%q", tt.args, done, code, out.String(), errOut.String())
		}
	}
}

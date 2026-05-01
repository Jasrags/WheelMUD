package telnet

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadCSI(t *testing.T) {
	tests := []struct {
		in   string
		want CSIOp
	}{
		// Note: the ESC byte is already consumed before ReadCSI runs, so
		// inputs start at the byte that follows.
		{"[A", CSIUp},
		{"[B", CSIDown},
		{"[C", CSIRight},
		{"[D", CSILeft},
		{"[H", CSIHome},
		{"[F", CSIEnd},
		{"[1~", CSIHome},
		{"[7~", CSIHome},
		{"[4~", CSIEnd},
		{"[8~", CSIEnd},
		{"[3~", CSIDelete},
		{"[5~", CSIUnknown}, // page up — not handled, but consumed
		{"[2;5R", CSIUnknown},
		{"OA", CSIUp},
		{"OD", CSILeft},
		{"OH", CSIHome},
		{"x", CSIUnknown}, // not a CSI introducer
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tt.in))
			got, err := ReadCSI(r)
			if err != nil {
				t.Fatalf("ReadCSI err: %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadCSI(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestReadCSIBoundedLength(t *testing.T) {
	// 33 digits of params with no final byte — must error rather than
	// consuming forever.
	in := "[" + strings.Repeat("1", 33)
	r := bufio.NewReader(strings.NewReader(in))
	if _, err := ReadCSI(r); err == nil {
		t.Fatal("expected error on overlong CSI body, got nil")
	}
}

package telnet

import "testing"

func TestDetectColorLevel(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"xterm-direct", ColorLevelTrueColor},
		{"xterm-truecolor", ColorLevelTrueColor},
		{"foo-truecolor", ColorLevelTrueColor},
		{"foo-24bit", ColorLevelTrueColor},
		{"xterm-256color", ColorLevel256},
		{"screen-256color", ColorLevel256},
		{"mudlet", ColorLevel256},
		{"xterm", ColorLevel16},
		{"vt100", ColorLevel16},
		{"dumb", ColorLevelNone},
		{"unknown", ColorLevelNone},
		{"weird-color-thing", ColorLevel16},
		{"", ColorLevel16},
	}
	for _, tc := range tests {
		if got := DetectColorLevel(tc.in); got != tc.want {
			t.Errorf("DetectColorLevel(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestRenderRGBFG_PerLevel(t *testing.T) {
	tests := []struct {
		level   int
		r, g, b int
		want    string
	}{
		{ColorLevelNone, 255, 0, 0, ""},
		{ColorLevelTrueColor, 255, 128, 0, "\x1b[38;2;255;128;0m"},
		{ColorLevelTrueColor, -5, 9999, 50, "\x1b[38;2;0;255;50m"}, // clamped
		{ColorLevel256, 0, 0, 0, "\x1b[38;5;16m"},
		{ColorLevel256, 255, 255, 255, "\x1b[38;5;231m"},
		{ColorLevel16, 200, 0, 0, "\x1b[91m"},   // bright red
		{ColorLevel16, 0, 0, 0, "\x1b[30m"},     // black
		{ColorLevel16, 0, 200, 200, "\x1b[96m"}, // bright cyan
	}
	for _, tc := range tests {
		got := RenderRGBFG(tc.level, tc.r, tc.g, tc.b)
		if got != tc.want {
			t.Errorf("RenderRGBFG(%d,%d,%d,%d) = %q, want %q", tc.level, tc.r, tc.g, tc.b, got, tc.want)
		}
	}
}

func TestRenderRGBBG_TrueColor(t *testing.T) {
	got := RenderRGBBG(ColorLevelTrueColor, 10, 20, 30)
	want := "\x1b[48;2;10;20;30m"
	if got != want {
		t.Errorf("RenderRGBBG = %q, want %q", got, want)
	}
}

func TestSGR(t *testing.T) {
	if got := SGR(); got != "\x1b[0m" {
		t.Errorf("SGR() = %q, want reset", got)
	}
	if got := SGR(ANSI_BOLD, ANSI_UNDERLINE); got != "\x1b[1;4m" {
		t.Errorf("SGR(bold,underline) = %q", got)
	}
}

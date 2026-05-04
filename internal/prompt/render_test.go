package prompt

import "testing"

func TestRender(t *testing.T) {
	full := Vars{
		HPCur: 7, HPMax: 10,
		MnCur: 2, MnMax: 5,
		MvCur: 3, MvMax: 4,
		RoomName: "Vast Glade",
		Gold:     "5gc 2sp",
		Target:   "trolloc",
	}
	cases := []struct {
		name, tmpl, want string
		v                Vars
	}{
		{"empty", "", "", full},
		{"literal", "> ", "> ", full},
		{"hp_cur", "%h", "7", full},
		{"hp_max", "%H", "10", full},
		{"mana", "%m/%M", "2/5", full},
		{"move", "%v/%V", "3/4", full},
		{"room", "%r", "Vast Glade", full},
		{"gold", "%g", "5gc 2sp", full},
		{"target", "%t", "trolloc", full},
		{"escape_pct", "100%%", "100%", full},
		{"unknown_passthru", "%z", "%z", full},
		{"trailing_pct", "hp%", "hp%", full},
		{"realistic", "<%h/%H hp> ", "<7/10 hp> ", full},
		{"combo", "[%r] %h/%H %g> ", "[Vast Glade] 7/10 5gc 2sp> ", full},
		{"zero_values", "<%h/%H hp>", "<0/0 hp>", Vars{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Render(tc.tmpl, tc.v)
			if got != tc.want {
				t.Fatalf("Render(%q) = %q, want %q", tc.tmpl, got, tc.want)
			}
		})
	}
}

func TestNeedsRoom(t *testing.T) {
	cases := []struct {
		tmpl string
		want bool
	}{
		{"", false},
		{"<%h/%H>", false},
		{"%r", true},
		{"[%r] %h", true},
		{"%%r", false}, // escaped literal — `%%` consumes the first `%`,
		// leaving `r` as a literal char, not a token.
	}
	for _, tc := range cases {
		if got := NeedsRoom(tc.tmpl); got != tc.want {
			t.Errorf("NeedsRoom(%q) = %v, want %v", tc.tmpl, got, tc.want)
		}
	}
}

func TestNeedsGold(t *testing.T) {
	if NeedsGold("<%h>") {
		t.Errorf("NeedsGold without %%g should be false")
	}
	if !NeedsGold("%g") {
		t.Errorf("NeedsGold with %%g should be true")
	}
}

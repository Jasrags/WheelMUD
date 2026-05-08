package channeling

import (
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

func TestRefreshIfDue(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	mkSlots := func(curs [10]int8) [10]creature.SlotPool {
		var s [10]creature.SlotPool
		for i := range s {
			s[i] = creature.SlotPool{Cur: curs[i], Max: 4}
		}
		return s
	}

	tests := []struct {
		name      string
		c         *creature.Channeling
		now       time.Time
		want      bool
		wantCur   int8 // expected value of Slots[0].Cur after
		wantStamp bool // whether LastSlotRefreshAt should equal now
	}{
		{
			name: "nil channeler is no-op",
			c:    nil,
			now:  t0,
			want: false,
		},
		{
			name: "stilled never refreshes",
			c: &creature.Channeling{
				Stilled: true,
				Slots:   mkSlots([10]int8{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}),
			},
			now:     t0,
			want:    false,
			wantCur: 0,
		},
		{
			name: "first pulse with zero timestamp refills",
			c: &creature.Channeling{
				Slots: mkSlots([10]int8{2, 1, 0, 0, 0, 0, 0, 0, 0, 0}),
			},
			now:       t0,
			want:      true,
			wantCur:   4,
			wantStamp: true,
		},
		{
			name: "not due yet (4h since last refresh)",
			c: &creature.Channeling{
				LastSlotRefreshAt: t0.Add(-4 * time.Hour),
				Slots:             mkSlots([10]int8{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}),
			},
			now:     t0,
			want:    false,
			wantCur: 1,
		},
		{
			name: "due exactly at 8h refills",
			c: &creature.Channeling{
				LastSlotRefreshAt: t0.Add(-RefreshInterval),
				Slots:             mkSlots([10]int8{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}),
			},
			now:       t0,
			want:      true,
			wantCur:   4,
			wantStamp: true,
		},
		{
			name: "due restamps even with full slots",
			c: &creature.Channeling{
				LastSlotRefreshAt: t0.Add(-9 * time.Hour),
				Slots:             mkSlots([10]int8{4, 4, 4, 4, 4, 4, 4, 4, 4, 4}),
			},
			now:       t0,
			want:      true,
			wantCur:   4,
			wantStamp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RefreshIfDue(tt.c, tt.now)
			if got != tt.want {
				t.Fatalf("RefreshIfDue=%v want %v", got, tt.want)
			}
			if tt.c == nil {
				return
			}
			if tt.c.Slots[0].Cur != tt.wantCur {
				t.Fatalf("Slots[0].Cur=%d want %d", tt.c.Slots[0].Cur, tt.wantCur)
			}
			if tt.wantStamp && !tt.c.LastSlotRefreshAt.Equal(tt.now) {
				t.Fatalf("timestamp not stamped: %v", tt.c.LastSlotRefreshAt)
			}
		})
	}
}

func TestAccrueMadness(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		c    *creature.Channeling
		want bool
		got  func(*creature.Channeling) int16
		val  int16
	}{
		{
			name: "nil channeler is no-op",
			c:    nil,
			want: false,
			got:  func(c *creature.Channeling) int16 { return 0 },
		},
		{
			name: "unembraced does not accrue",
			c: &creature.Channeling{
				GenderSource: creature.SourceSaidin,
				Embraced:     false,
				Madness:      5,
			},
			want: false,
			got:  func(c *creature.Channeling) int16 { return c.Madness },
			val:  5,
		},
		{
			name: "embraced saidar (female) does not accrue",
			c: &creature.Channeling{
				GenderSource: creature.SourceSaidar,
				Embraced:     true,
				Madness:      0,
			},
			want: false,
			got:  func(c *creature.Channeling) int16 { return c.Madness },
			val:  0,
		},
		{
			name: "embraced saidin accrues",
			c: &creature.Channeling{
				GenderSource: creature.SourceSaidin,
				Embraced:     true,
				Madness:      10,
			},
			want: true,
			got:  func(c *creature.Channeling) int16 { return c.Madness },
			val:  11,
		},
		{
			name: "stilled does not accrue even when embraced",
			c: &creature.Channeling{
				GenderSource: creature.SourceSaidin,
				Embraced:     true,
				Stilled:      true,
				Madness:      0,
			},
			want: false,
			got:  func(c *creature.Channeling) int16 { return c.Madness },
			val:  0,
		},
		{
			name: "clamps at int16 max",
			c: &creature.Channeling{
				GenderSource: creature.SourceSaidin,
				Embraced:     true,
				Madness:      MadnessMax,
			},
			want: false,
			got:  func(c *creature.Channeling) int16 { return c.Madness },
			val:  MadnessMax,
		},
		{
			name: "near-max clamps without overflow",
			c: &creature.Channeling{
				GenderSource: creature.SourceSaidin,
				Embraced:     true,
				Madness:      MadnessMax - 1,
			},
			want: true,
			got:  func(c *creature.Channeling) int16 { return c.Madness },
			val:  MadnessMax,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AccrueMadness(tt.c, now)
			if got != tt.want {
				t.Fatalf("AccrueMadness=%v want %v", got, tt.want)
			}
			if tt.c == nil {
				return
			}
			if v := tt.got(tt.c); v != tt.val {
				t.Fatalf("Madness=%d want %d", v, tt.val)
			}
		})
	}
}

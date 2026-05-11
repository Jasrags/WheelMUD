package combat

import (
	"reflect"
	"testing"
)

func TestIterativeBonusesFor(t *testing.T) {
	cases := []struct {
		name string
		bab  int16
		want []int16
	}{
		{"zero BAB still swings once", 0, []int16{0}},
		{"negative BAB still swings once", -3, []int16{0}},
		{"BAB 1 single swing", 1, []int16{0}},
		{"BAB 5 single swing", 5, []int16{0}},
		{"BAB 6 unlocks second swing", 6, []int16{0, -5}},
		{"BAB 10 still two swings", 10, []int16{0, -5}},
		{"BAB 11 unlocks third swing", 11, []int16{0, -5, -10}},
		{"BAB 15 still three swings", 15, []int16{0, -5, -10}},
		{"BAB 16 unlocks fourth swing", 16, []int16{0, -5, -10, -15}},
		{"BAB 20 caps at four swings", 20, []int16{0, -5, -10, -15}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IterativeBonusesFor(tc.bab)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("IterativeBonusesFor(%d) = %v, want %v", tc.bab, got, tc.want)
			}
		})
	}
}

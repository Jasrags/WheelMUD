package mode

import (
	"testing"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
)

func TestDeriveLevel1Vitals(t *testing.T) {
	score := func(v int8) creature.AbilityScore {
		return creature.AbilityScore{Current: v, Max: v}
	}

	tests := []struct {
		name        string
		hitDie      int
		fort, ref   chargen.SaveProgression
		will        chargen.SaveProgression
		str, dex    int8
		con, intel  int8
		wis, cha    int8
		wantHP      int32
		wantDefense int16
		wantFort    int16
		wantRef     int16
		wantWill    int16
	}{
		{
			name:   "armsman_baseline",
			hitDie: 10, fort: chargen.SaveHigh, ref: chargen.SaveLow, will: chargen.SaveLow,
			str: 13, dex: 15, con: 15, intel: 11, wis: 9, cha: 8,
			// Con mod = +2 → HP 12; Dex mod = +2 → Def 12;
			// Fort 2+2=4, Ref 0+2=2, Will 0+(-1)=-1
			wantHP: 12, wantDefense: 12, wantFort: 4, wantRef: 2, wantWill: -1,
		},
		{
			name:   "wilder_high_will",
			hitDie: 6, fort: chargen.SaveLow, ref: chargen.SaveLow, will: chargen.SaveHigh,
			str: 8, dex: 12, con: 10, intel: 14, wis: 16, cha: 13,
			// Con +0 → HP 6; Dex +1 → Def 11;
			// Fort 0+0=0, Ref 0+1=1, Will 2+3=5
			wantHP: 6, wantDefense: 11, wantFort: 0, wantRef: 1, wantWill: 5,
		},
		{
			name:   "hp_floor_at_1_when_negative_con",
			hitDie: 4, fort: chargen.SaveLow, ref: chargen.SaveLow, will: chargen.SaveLow,
			str: 10, dex: 10, con: 1, intel: 10, wis: 10, cha: 10,
			// Con mod = -5; 4 + (-5) = -1 → floored to 1
			wantHP: 1, wantDefense: 10, wantFort: -5, wantRef: 0, wantWill: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := &chargen.Class{
				HitDie:   tt.hitDie,
				SaveFort: tt.fort,
				SaveRef:  tt.ref,
				SaveWill: tt.will,
			}
			ab := creature.Abilities{
				Str: score(tt.str), Dex: score(tt.dex), Con: score(tt.con),
				Int: score(tt.intel), Wis: score(tt.wis), Cha: score(tt.cha),
			}
			hp, def, saves := deriveLevel1Vitals(cl, ab)
			if hp != tt.wantHP {
				t.Errorf("HP = %d, want %d", hp, tt.wantHP)
			}
			if def != tt.wantDefense {
				t.Errorf("Defense = %d, want %d", def, tt.wantDefense)
			}
			if saves.Fort != tt.wantFort {
				t.Errorf("Fort = %d, want %d", saves.Fort, tt.wantFort)
			}
			if saves.Ref != tt.wantRef {
				t.Errorf("Ref = %d, want %d", saves.Ref, tt.wantRef)
			}
			if saves.Will != tt.wantWill {
				t.Errorf("Will = %d, want %d", saves.Will, tt.wantWill)
			}
		})
	}
}

package telnet

import (
	"bytes"
	"testing"
)

func TestEncodeMSSP(t *testing.T) {
	tests := []struct {
		name string
		vars []MSSPVar
		want []byte
	}{
		{
			name: "empty",
			vars: nil,
			want: []byte{TELNET_IAC, TELNET_SB, TELNET_OPT_MSSP, TELNET_IAC, TELNET_SE},
		},
		{
			name: "single",
			vars: []MSSPVar{{Name: "NAME", Value: "WheelMUD"}},
			want: []byte{
				TELNET_IAC, TELNET_SB, TELNET_OPT_MSSP,
				MSSP_VAR, 'N', 'A', 'M', 'E',
				MSSP_VAL, 'W', 'h', 'e', 'e', 'l', 'M', 'U', 'D',
				TELNET_IAC, TELNET_SE,
			},
		},
		{
			name: "multi",
			vars: []MSSPVar{
				{Name: "NAME", Value: "WheelMUD"},
				{Name: "PLAYERS", Value: "42"},
			},
			want: []byte{
				TELNET_IAC, TELNET_SB, TELNET_OPT_MSSP,
				MSSP_VAR, 'N', 'A', 'M', 'E',
				MSSP_VAL, 'W', 'h', 'e', 'e', 'l', 'M', 'U', 'D',
				MSSP_VAR, 'P', 'L', 'A', 'Y', 'E', 'R', 'S',
				MSSP_VAL, '4', '2',
				TELNET_IAC, TELNET_SE,
			},
		},
		{
			name: "iac in value is escaped",
			vars: []MSSPVar{{Name: "X", Value: string([]byte{'a', TELNET_IAC, 'b'})}},
			want: []byte{
				TELNET_IAC, TELNET_SB, TELNET_OPT_MSSP,
				MSSP_VAR, 'X',
				MSSP_VAL, 'a', TELNET_IAC, TELNET_IAC, 'b',
				TELNET_IAC, TELNET_SE,
			},
		},
		{
			name: "empty value",
			vars: []MSSPVar{{Name: "WEBSITE", Value: ""}},
			want: []byte{
				TELNET_IAC, TELNET_SB, TELNET_OPT_MSSP,
				MSSP_VAR, 'W', 'E', 'B', 'S', 'I', 'T', 'E',
				MSSP_VAL,
				TELNET_IAC, TELNET_SE,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EncodeMSSP(tc.vars)
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("EncodeMSSP\n got:  %v\n want: %v", got, tc.want)
			}
		})
	}
}

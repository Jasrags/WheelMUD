package telnet

// MSSPVar is one variable in an MSSP response (mssp.org spec). Name is
// the canonical key (e.g. "NAME", "PLAYERS"); Value is the rendered
// string. The wire form is `MSSP_VAR <name> MSSP_VAL <value>`. V1
// doesn't emit array values — every variable in the WheelMUD set is
// single-valued — so the struct mirrors that.
type MSSPVar struct {
	Name  string
	Value string
}

// EncodeMSSP renders an MSSP response as the full wire bytes:
//
//	IAC SB MSSP (MSSP_VAR <name> MSSP_VAL <value>)+ IAC SE
//
// Any 0xFF byte in a name or value is escaped as `IAC IAC` per RFC 855
// — the surrounding SB/SE framing terminates on a bare IAC byte.
// MSSP_VAR (1) and MSSP_VAL (2) are framing bytes and are not escaped
// here; the spec only requires escaping of IAC itself.
//
// An empty input still emits a well-formed (but variable-less) block —
// the caller is expected to guard against that.
func EncodeMSSP(vars []MSSPVar) []byte {
	out := make([]byte, 0, 6+16*len(vars))
	out = append(out, TELNET_IAC, TELNET_SB, TELNET_OPT_MSSP)
	for _, v := range vars {
		out = append(out, MSSP_VAR)
		out = appendMSSPField(out, v.Name)
		out = append(out, MSSP_VAL)
		out = appendMSSPField(out, v.Value)
	}
	out = append(out, TELNET_IAC, TELNET_SE)
	return out
}

func appendMSSPField(out []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == TELNET_IAC {
			out = append(out, TELNET_IAC, TELNET_IAC)
			continue
		}
		out = append(out, b)
	}
	return out
}

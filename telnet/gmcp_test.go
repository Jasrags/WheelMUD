package telnet

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
)

func TestEncodeGMCPFrame_RoundTrip(t *testing.T) {
	body := struct {
		HP int `json:"hp"`
	}{HP: 42}
	got, err := encodeGMCPFrame("Char.Vitals", body)
	if err != nil {
		t.Fatalf("encodeGMCPFrame: %v", err)
	}
	if !bytes.HasPrefix(got, []byte{TELNET_IAC, TELNET_SB, TELNET_OPT_GMCP}) {
		t.Fatalf("missing IAC SB GMCP prefix: %v", got)
	}
	if !bytes.HasSuffix(got, []byte{TELNET_IAC, TELNET_SE}) {
		t.Fatalf("missing IAC SE suffix: %v", got)
	}
	if !bytes.Contains(got, []byte("Char.Vitals {")) {
		t.Fatalf("missing pkg+space+body: %q", got)
	}
	if !bytes.Contains(got, []byte(`"hp":42`)) {
		t.Fatalf("missing body JSON: %q", got)
	}
}

func TestAppendGMCPField_IACEscape(t *testing.T) {
	// The escape branch in appendGMCPField is defense-in-depth: JSON
	// marshaling never emits a literal 0xFF in normal payloads (it
	// encodes high bytes as \u escapes), but a future code path that
	// sends raw bytes through the frame must round-trip IAC IAC.
	in := []byte{'a', TELNET_IAC, 'b', TELNET_IAC, TELNET_IAC}
	got := appendGMCPField(nil, in)
	want := []byte{'a', TELNET_IAC, TELNET_IAC, 'b', TELNET_IAC, TELNET_IAC, TELNET_IAC, TELNET_IAC}
	if !bytes.Equal(got, want) {
		t.Fatalf("appendGMCPField\n got:  %v\n want: %v", got, want)
	}
}

func TestNegotiateTelnet_OffersWillGMCP(t *testing.T) {
	s, peer := newPipeSession(t)

	gotCh := make(chan []byte, 1)
	go func() { gotCh <- drainPeerBytes(t, peer, 256) }()

	if err := NegotiateTelnet(s.Conn); err != nil {
		t.Fatalf("NegotiateTelnet: %v", err)
	}
	s.Conn.Close()
	out := <-gotCh
	want := []byte{TELNET_IAC, TELNET_WILL, TELNET_OPT_GMCP}
	if !bytes.Contains(out, want) {
		t.Fatalf("initial offer missing WILL GMCP: %v", out)
	}
}

func TestHandleOptionNegotiation_DOGMCPFlipsFlag(t *testing.T) {
	s, _ := newPipeSession(t)
	if s.GMCPEnabled() {
		t.Fatal("expected GMCPEnabled=false before negotiation")
	}
	r := bufio.NewReader(bytes.NewReader([]byte{TELNET_DO, TELNET_OPT_GMCP}))
	if _, _, err := ReadIAC(s, r); err != nil {
		t.Fatalf("ReadIAC: %v", err)
	}
	if !s.GMCPEnabled() {
		t.Fatal("expected GMCPEnabled=true after DO GMCP")
	}
}

func TestHandleGMCPSubnegotiation_DispatchesToHandler(t *testing.T) {
	s, _ := newPipeSession(t)
	var gotPkg string
	var gotBody []byte
	s.GMCPHandler = func(_ *Session, pkg string, body []byte) {
		gotPkg = pkg
		gotBody = body
	}
	// `Char.Vitals {"hp":1}`
	payload := append([]byte("Char.Vitals "), []byte(`{"hp":1}`)...)
	handleGMCPSubnegotiation(s, payload)
	if gotPkg != "Char.Vitals" {
		t.Fatalf("pkg = %q, want Char.Vitals", gotPkg)
	}
	var parsed map[string]int
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("body parse: %v (raw=%q)", err, gotBody)
	}
	if parsed["hp"] != 1 {
		t.Fatalf("hp = %d, want 1", parsed["hp"])
	}
}

func TestHandleGMCPSubnegotiation_NoSpaceMeansNullBody(t *testing.T) {
	s, _ := newPipeSession(t)
	var gotPkg string
	var gotBody []byte
	s.GMCPHandler = func(_ *Session, pkg string, body []byte) {
		gotPkg = pkg
		gotBody = body
	}
	handleGMCPSubnegotiation(s, []byte("Core.Ping"))
	if gotPkg != "Core.Ping" {
		t.Fatalf("pkg = %q, want Core.Ping", gotPkg)
	}
	if string(gotBody) != "null" {
		t.Fatalf("body = %q, want null", gotBody)
	}
}

func TestSessionWriteGMCP_NoOpWhenDisabled(t *testing.T) {
	s, peer := newPipeSession(t)
	// Don't enable GMCP. Write should silently no-op without touching
	// the wire — confirm by closing the peer and asserting an immediate
	// 0-byte read.
	if err := s.WriteGMCP("Char.Vitals", CharVitalsForTest{HP: 1, MaxHP: 1}); err != nil {
		t.Fatalf("WriteGMCP: %v", err)
	}
	s.Conn.Close()
	buf := make([]byte, 16)
	n, _ := peer.Read(buf)
	if n != 0 {
		t.Fatalf("WriteGMCP wrote %d bytes when disabled", n)
	}
}

// CharVitalsForTest mirrors the gmcp.CharVitals shape without
// importing internal/gmcp (which would cycle through telnet).
type CharVitalsForTest struct {
	HP    int32 `json:"hp"`
	MaxHP int32 `json:"maxhp"`
}

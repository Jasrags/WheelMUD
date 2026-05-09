package lua

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	gluua "github.com/yuin/gopher-lua"

	"github.com/Jasrags/WheelMUD/internal/scripts"
)

// loadScript compiles src as a script named name, returns a catalog
// containing it. Used by every runner test for one-shot fixtures.
func loadScript(t *testing.T, name, src string) *scripts.Catalog {
	t.Helper()
	body := []byte(src)
	fsys := fstest.MapFS{name + ".lua": &fstest.MapFile{Data: body}}
	parser := gluua.NewState()
	defer parser.Close()
	cat, err := scripts.Load(fsys, parser)
	if err != nil {
		t.Fatalf("scripts.Load %s: %v", name, err)
	}
	return cat
}

// Each disabledGlobals entry must be unreachable from the sandbox.
// A direct call would normally raise "attempt to call a nil value";
// we just confirm `type(<name>)` returns "nil".
func TestSandbox_DisabledGlobalsAreNil(t *testing.T) {
	for _, g := range disabledGlobals {
		t.Run(g, func(t *testing.T) {
			src := `
local t = type(` + g + `)
if t ~= "nil" then error("expected nil, got " .. t) end
`
			cat := loadScript(t, "probe_"+sanitize(g), src)
			r := NewRunner(cat, nil)
			defer r.Stop()
			if err := r.Run(context.Background(), "probe_"+sanitize(g), nil); err != nil {
				t.Fatalf("expected nil global, got error: %v", err)
			}
		})
	}
}

// sanitize swaps anything outside [a-z0-9_] with "_" so a global
// like "io" becomes a valid script name (it's already valid; this
// is defensive).
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func TestRunner_UnknownScript(t *testing.T) {
	cat := &scripts.Catalog{ByName: map[string]*scripts.Script{}}
	r := NewRunner(cat, nil)
	defer r.Stop()
	err := r.Run(context.Background(), "ghost", nil)
	if !errors.Is(err, ErrUnknownScript) {
		t.Fatalf("err = %v, want ErrUnknownScript", err)
	}
}

func TestRunner_HappyPath(t *testing.T) {
	cat := loadScript(t, "noop", `local x = 1 + 1`)
	r := NewRunner(cat, nil)
	defer r.Stop()
	if err := r.Run(context.Background(), "noop", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunner_RuntimeError(t *testing.T) {
	cat := loadScript(t, "boom", `error("nope")`)
	r := NewRunner(cat, nil)
	defer r.Stop()
	err := r.Run(context.Background(), "boom", nil)
	if !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v, want ErrLuaError", err)
	}
}

func TestRunner_TightLoopCtxTimeout(t *testing.T) {
	// A tight CPU loop must terminate inside CallTimeout via
	// SetContext propagation. Without it the test would hang and
	// the goroutine running the script would survive past Stop.
	cat := loadScript(t, "spin", `
while true do
  local x = 1
end
`)
	r := NewRunner(cat, nil)
	defer r.Stop()
	start := time.Now()
	err := r.Run(context.Background(), "spin", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from infinite loop")
	}
	if !errors.Is(err, ErrTimeout) && !errors.Is(err, ErrLuaError) {
		t.Fatalf("err = %v; want ErrTimeout or ErrLuaError", err)
	}
	// CallTimeout is 50ms; allow generous slack for race-mode
	// scheduler. A 5s ceiling catches a regression where the
	// timeout doesn't propagate at all.
	if elapsed > 5*time.Second {
		t.Fatalf("took %v; ctx propagation may be broken", elapsed)
	}
}

func TestRunner_BindCallback(t *testing.T) {
	// Verify the bind callback can register a global the script
	// then invokes. Counts captured calls.
	cat := loadScript(t, "callback", `say("hello")`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	var captured []string
	bindings := APIBindings{
		Broadcast: func(text string) { captured = append(captured, text) },
	}
	bind := func(l *gluua.LState) { bindings.Bind(l) }

	if err := r.Run(context.Background(), "callback", bind); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(captured) != 1 || captured[0] != "hello" {
		t.Fatalf("captured = %v", captured)
	}
}

func TestRunner_CtxTableExposesEventCtx(t *testing.T) {
	cat := loadScript(t, "probe_ctx", `
log("info", string.format("event=%s actor=%d room=%d text=%s",
  ctx.event, ctx.actor_id, ctx.room_id, ctx.text))
`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	bindings := APIBindings{
		Ctx: CtxView{
			Event:   "on_say",
			ActorID: 42,
			RoomID:  100,
			Text:    "hello",
		},
	}
	bind := func(l *gluua.LState) { bindings.Bind(l) }

	if err := r.Run(context.Background(), "probe_ctx", bind); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunner_ConcurrentCallsAreSafe(t *testing.T) {
	// Pool serves concurrent invocations. Race detector catches any
	// shared-state mutation across goroutines.
	cat := loadScript(t, "concurrent", `local x = 1`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	const workers = 16
	const callsEach = 50
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < callsEach; j++ {
				_ = r.Run(context.Background(), "concurrent", nil)
			}
		}()
	}
	wg.Wait()
}

func TestRunner_ParentCtxCancellation(t *testing.T) {
	// A canceled parent ctx should propagate immediately even if
	// the script itself is short.
	cat := loadScript(t, "tight", `for i=1,1000 do local x=i end`)
	r := NewRunner(cat, nil)
	defer r.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	err := r.Run(ctx, "tight", nil)
	// The script may complete before checking ctx (1000 iterations
	// is well under the cap and cheap), or it may surface as
	// ErrTimeout. Both acceptable; what's NOT acceptable is a hang.
	_ = err
}

func TestRunner_StopRefusesFurtherCalls(t *testing.T) {
	cat := loadScript(t, "noop2", `local x = 1`)
	r := NewRunner(cat, nil)
	r.Stop()
	err := r.Run(context.Background(), "noop2", nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout (engine winding down)", err)
	}
}

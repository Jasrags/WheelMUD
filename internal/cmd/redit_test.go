package cmd

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// pushREditCalls is a stub PushREditFn that records what would have
// been pushed so verb tests can assert "redit pushed the editor for
// room X" without pulling internal/mode into internal/cmd's test set.
type pushREditCalls struct {
	calls []repo.Room
}

func (p *pushREditCalls) push(_ *telnet.Session, r repo.Room) error {
	p.calls = append(p.calls, r)
	return nil
}

func TestREdit_NoArg_UsesCurrentRoom(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	room := rooms.Insert(repo.Room{ID: 10, ExternalID: "plaza", ZoneID: 7, Name: "Plaza"})
	pushes := &pushREditCalls{}

	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	s.SetCurrentRoom(room.ID)

	runCmd(t, NewREdit(rooms, pushes.push), s, "")
	if len(pushes.calls) != 1 {
		t.Fatalf("push count = %d, want 1", len(pushes.calls))
	}
	if pushes.calls[0].ID != room.ID {
		t.Fatalf("pushed room ID = %d, want %d", pushes.calls[0].ID, room.ID)
	}
}

func TestREdit_WithIDArg_ResolvesNumeric(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	room := rooms.Insert(repo.Room{ID: 99, ExternalID: "vault", ZoneID: 3, Name: "Vault"})
	pushes := &pushREditCalls{}

	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	s.SetCurrentRoom(1)

	runCmd(t, NewREdit(rooms, pushes.push), s, "99")
	if len(pushes.calls) != 1 || pushes.calls[0].ID != room.ID {
		t.Fatalf("expected push for room 99, got %+v", pushes.calls)
	}
}

func TestREdit_WithExternalArg_Resolves(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	room := rooms.Insert(repo.Room{ID: 99, ExternalID: "vault.inner", ZoneID: 3})
	pushes := &pushREditCalls{}

	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	s.SetCurrentRoom(1)

	runCmd(t, NewREdit(rooms, pushes.push), s, "vault.inner")
	if len(pushes.calls) != 1 || pushes.calls[0].ID != room.ID {
		t.Fatalf("expected push for vault.inner, got %+v", pushes.calls)
	}
}

func TestREdit_MissingRoomRefuses(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	pushes := &pushREditCalls{}

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	s.SetCurrentRoom(1)

	runCmd(t, NewREdit(rooms, pushes.push), s, "ghost")
	if len(pushes.calls) != 0 {
		t.Fatalf("push should not happen for missing room: %+v", pushes.calls)
	}
	if !strings.Contains(out.String(), "No such room") {
		t.Fatalf("missing refusal: %q", out.String())
	}
}

func TestREdit_PlayerWithoutGrantRefuses(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	room := rooms.Insert(repo.Room{ID: 10, ExternalID: "plaza", ZoneID: 7})
	pushes := &pushREditCalls{}

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer // no grants seeded
	s.SetCurrentRoom(room.ID)

	runCmd(t, NewREdit(rooms, pushes.push), s, "")
	if len(pushes.calls) != 0 {
		t.Fatalf("push should not happen without grant: %+v", pushes.calls)
	}
	if !strings.Contains(out.String(), "do not have permission") {
		t.Fatalf("missing refusal: %q", out.String())
	}
}

func TestREdit_BuilderWithMatchingGrantPushes(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	room := rooms.Insert(repo.Room{ID: 10, ExternalID: "plaza", ZoneID: 7})
	pushes := &pushREditCalls{}

	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.SetBuilderZones(map[int64]struct{}{room.ZoneID: {}})
	s.SetCurrentRoom(room.ID)

	runCmd(t, NewREdit(rooms, pushes.push), s, "")
	if len(pushes.calls) != 1 {
		t.Fatalf("builder with grant: push count = %d, want 1", len(pushes.calls))
	}
}

func TestREdit_BuilderWithWrongZoneRefuses(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	room := rooms.Insert(repo.Room{ID: 10, ExternalID: "plaza", ZoneID: 7})
	pushes := &pushREditCalls{}

	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	// Grant on a DIFFERENT zone — must not extend to this room.
	s.SetBuilderZones(map[int64]struct{}{999: {}})
	s.SetCurrentRoom(room.ID)

	runCmd(t, NewREdit(rooms, pushes.push), s, "")
	if len(pushes.calls) != 0 {
		t.Fatalf("wrong-zone grant should not authorise: %+v", pushes.calls)
	}
	if !strings.Contains(out.String(), "do not have permission") {
		t.Fatalf("missing refusal: %q", out.String())
	}
}

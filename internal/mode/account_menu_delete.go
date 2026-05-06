package mode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// handleDeleteEnter shows the delete picker. Empty rosters route back
// to root (rootMenu hides the option already; this is defensive).
func (m *AccountMenu) handleDeleteEnter(s *telnet.Session) error {
	if len(m.chars) == 0 {
		return m.returnToRoot(s)
	}
	m.step = accountStepDeletePicker
	return m.writeDeletePicker(s)
}

func (m *AccountMenu) writeDeletePicker(s *telnet.Session) error {
	if err := display.SectionHeader(s, "Delete a character"); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("  Pick a character to delete (cannot be undone):\r\n")); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte(m.charListBlock())); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("\r\n  [B]ack\r\n")); err != nil {
		return err
	}
	return s.WriteRaw([]byte(fmt.Sprintf("[1-%d] >\r\n", len(m.chars))))
}

func (m *AccountMenu) handleDeletePickerLine(s *telnet.Session, line string) error {
	if isBack(line) {
		return m.returnToRoot(s)
	}
	idx, err := parsePositiveIndex(strings.TrimSpace(line), len(m.chars))
	if err != nil {
		if werr := writeError(s,
			fmt.Sprintf("Invalid choice. [1-%d] or [B]ack.", len(m.chars))); werr != nil {
			return werr
		}
		return nil
	}
	target := m.chars[idx]
	if m.session != nil {
		if active := m.session.FindByCharacterName(target.Name); active != nil {
			if werr := writeError(s, "That character is currently logged in. Try again later."); werr != nil {
				return werr
			}
			return m.writeDeletePicker(s)
		}
	}
	m.pendingDelete = &target
	m.step = accountStepConfirmDelete
	return m.writeDeleteConfirm(s, target)
}

func (m *AccountMenu) writeDeleteConfirm(s *telnet.Session, target repo.Character) error {
	if err := display.SectionHeader(s, "Delete a character"); err != nil {
		return err
	}
	last := "never"
	if target.LastPlayedAt != nil && !target.LastPlayedAt.IsZero() {
		last = target.LastPlayedAt.In(m.displayLocation()).Format("2006-01-02")
	}
	if err := s.WriteRaw([]byte(fmt.Sprintf(
		"  %s  %s  lvl %d  last %s\r\n\r\n",
		target.Name, className(target), totalClassLevels(target.ClassLevels), last))); err != nil {
		return err
	}
	body := "  This cannot be undone. All carried items, equipment,\r\n" +
		"  and bank balance will be destroyed.\r\n\r\n" +
		"  Type the character's name exactly (case-sensitive) to confirm,\r\n" +
		"  or [B]ack to cancel.\r\n"
	return s.WriteRaw([]byte(body))
}

// handleConfirmDelete runs at accountStepConfirmDelete. cancel/blank/
// "back" returns to root; an exact (case-sensitive) name match
// executes the cascade. Mismatches repeat the prompt rather than
// auto-aborting so a typo doesn't lose progress, and so the deletion
// stays explicit.
func (m *AccountMenu) handleConfirmDelete(ctx context.Context, s *telnet.Session, line string) error {
	in := strings.TrimSpace(line)
	if in == "" || strings.EqualFold(in, "cancel") || strings.EqualFold(in, "abort") ||
		strings.EqualFold(in, "back") || strings.EqualFold(in, "b") {
		m.pendingDelete = nil
		return m.returnToRoot(s)
	}
	target := m.pendingDelete
	if target == nil {
		// Defensive: confirm step entered without a pending target.
		return m.returnToRoot(s)
	}
	if in != target.Name {
		if werr := writeError(s, "Names did not match. Type the character's name exactly, or [B]ack."); werr != nil {
			return werr
		}
		return nil
	}
	if err := m.executeDelete(ctx, s, *target); err != nil {
		m.pendingDelete = nil
		slog.Warn("account_menu: delete cascade failed",
			"account", s.AccountID, "character", target.ID, "err", err)
		if werr := writeError(s, "Could not delete that character. Try again later."); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	}
	m.pendingDelete = nil
	if err := display.OK(s, "Character deleted."); err != nil {
		return err
	}
	return m.refreshAndReturn(ctx, s, target.ID)
}

// executeDelete performs the application-level cascade for a character
// row: every owned item (top-level inventory plus everything nested
// inside containers they own) is deleted, then the character row
// itself, then an account-mode audit row is written. Order matters —
// ListAllOwnedTransitive before character.Delete keeps the BFS through
// items.parent_item_id valid; character.Delete is the last destructive
// step so a partial item failure leaves a deletable character.
//
// Refuses if m.items is nil (the items repo wasn't wired). The
// alternative — silently skipping the cascade and deleting the row —
// would orphan items.owner_character_id references, which is the
// invariant slice 1b exists to enforce. Production wiring always
// supplies items; this guard catches misconfiguration loudly.
//
// The live-session check is re-run here (in addition to the picker)
// to close the TOCTOU window between confirm prompt and execute.
// Single-session-per-account makes this almost impossible today, but
// cheap.
func (m *AccountMenu) executeDelete(ctx context.Context, s *telnet.Session, target repo.Character) error {
	if m.items == nil {
		return errors.New("item repo not wired; refusing to delete character without item cascade")
	}
	if m.session != nil {
		if active := m.session.FindByCharacterName(target.Name); active != nil {
			return fmt.Errorf("character %q is logged in", target.Name)
		}
	}
	owned, err := m.items.ListAllOwnedTransitive(ctx, target.ID)
	if err != nil {
		return fmt.Errorf("list owned items: %w", err)
	}
	for _, it := range owned {
		if err := m.items.Delete(ctx, it.ID); err != nil {
			return fmt.Errorf("delete item %d: %w", it.ID, err)
		}
	}
	if err := m.repo.Delete(ctx, target.ID); err != nil {
		return fmt.Errorf("delete character row: %w", err)
	}
	audit.RecordAccount(ctx, m.audits, s.AccountID, m.accountUsername,
		"delete-character", target.Name,
		fmt.Sprintf("id=%d level=%d", target.ID, totalClassLevels(target.ClassLevels)))
	return nil
}

// refreshAndReturn reloads the character roster from the repo so the
// post-delete root reflects state-of-the-world, then re-renders the
// root. deletedID is dropped from the cached roster on repo failure
// so the fallback render doesn't show the just-deleted character.
func (m *AccountMenu) refreshAndReturn(ctx context.Context, s *telnet.Session, deletedID int64) error {
	chars, err := m.repo.ListByAccount(ctx, s.AccountID)
	if err != nil {
		slog.Warn("account_menu: refresh ListByAccount failed",
			"account", s.AccountID, "err", err)
		filtered := m.chars[:0]
		for _, c := range m.chars {
			if c.ID != deletedID {
				filtered = append(filtered, c)
			}
		}
		m.chars = filtered
	} else {
		m.chars = chars
	}
	return m.returnToRoot(s)
}

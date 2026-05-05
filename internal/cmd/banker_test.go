package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// bankerFixture extends invFixture with a banker mob in room 1.
type bankerFixture struct {
	*invFixture
	mobs      *repo.MemoryMobInstanceRepo
	templates *repo.MemoryMobTemplateRepo
	bankers   *repo.MemoryBankerRepo
	audits    repo.AdminAuditRepo
	keeperTpl int64
	bankerID  int64
	hour      *fixedHour
}

func newBankerFixture(t *testing.T) *bankerFixture {
	t.Helper()
	inv := newInvFixture(t)

	templates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	bankers := repo.NewMemoryBankerRepo()

	tpl, err := templates.Create(context.Background(), creature.MobTemplate{
		ExternalID: "city.banker",
		Core:       creature.Core{Name: "Jain"},
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if _, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: tpl.ID,
		Core:       creature.Core{CurrentRoomID: 1},
	}); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	b, err := bankers.Create(context.Background(), repo.Banker{
		MobTemplateID: tpl.ID,
		// OpenHour == CloseHour → always open
	})
	if err != nil {
		t.Fatalf("create banker: %v", err)
	}

	hour := fixedHour(12)
	return &bankerFixture{
		invFixture: inv,
		mobs:       mobs,
		templates:  templates,
		bankers:    bankers,
		audits:     repo.NewMemoryAdminAuditRepo(),
		keeperTpl:  tpl.ID,
		bankerID:   b.ID,
		hour:       &hour,
	}
}

func (f *bankerFixture) balanceCmd() *telnet.Command {
	return NewBalance(f.characters, f.mobs, f.templates, f.bankers, f.hour)
}
func (f *bankerFixture) depositCmd() *telnet.Command {
	return NewDeposit(f.characters, f.mobs, f.templates, f.bankers, f.hour, f.audits)
}
func (f *bankerFixture) withdrawCmd() *telnet.Command {
	return NewWithdraw(f.characters, f.mobs, f.templates, f.bankers, f.hour, f.audits)
}

// setWealth stamps Alice's coin + bank balance directly via RecordCoin.
func (f *bankerFixture) setWealth(t *testing.T, coinCP, bankCP int64) {
	t.Helper()
	c, err := f.characters.FindByName(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("find alice: %v", err)
	}
	if err := f.characters.RecordCoin(context.Background(), c.ID,
		currency.Amount(coinCP), currency.Amount(bankCP)); err != nil {
		t.Fatalf("record coin: %v", err)
	}
}

func (f *bankerFixture) aliceWealth(t *testing.T) (carried, deposited currency.Amount) {
	t.Helper()
	c, err := f.characters.FindByName(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("find alice: %v", err)
	}
	return c.Coin, c.BankBalance
}

func TestBanker_BalanceWithoutBankerRefuses(t *testing.T) {
	f := newBankerFixture(t)
	// Drop alice into a room with no banker.
	f.alice.CurrentRoomID = 2

	runCmd(t, f.balanceCmd(), f.alice, "")
	if !strings.Contains(f.aOut.String(), "no banker here") {
		t.Fatalf("expected refusal, got %q", f.aOut.String())
	}
}

func TestBanker_BalanceShowsBoth(t *testing.T) {
	f := newBankerFixture(t)
	f.setWealth(t, 100, 500) // 1mk carried, 5mk on deposit

	runCmd(t, f.balanceCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "You carry") || !strings.Contains(out, "On deposit") {
		t.Fatalf("missing balance fields; got %q", out)
	}
}

func TestBanker_BalanceClosed(t *testing.T) {
	f := newBankerFixture(t)
	// Replace banker repo with a closed-window config (open 22-6, hour=12).
	bankers := repo.NewMemoryBankerRepo()
	if _, err := bankers.Create(context.Background(), repo.Banker{
		MobTemplateID: f.keeperTpl, OpenHour: 22, CloseHour: 6,
	}); err != nil {
		t.Fatalf("recreate banker: %v", err)
	}
	f.bankers = bankers

	runCmd(t, f.balanceCmd(), f.alice, "")
	if !strings.Contains(f.aOut.String(), "isn't taking customers") {
		t.Fatalf("expected closed message, got %q", f.aOut.String())
	}
}

func TestBanker_DepositSuccess(t *testing.T) {
	f := newBankerFixture(t)
	f.setWealth(t, 1000, 0) // 10mk carried, 0 deposit

	runCmd(t, f.depositCmd(), f.alice, "5mk")
	out := f.aOut.String()
	if !strings.Contains(out, "You deposit") {
		t.Fatalf("missing deposit echo; got %q", out)
	}
	carried, deposited := f.aliceWealth(t)
	if int64(carried) != 500 || int64(deposited) != 500 {
		t.Fatalf("balances wrong: carried=%d deposited=%d", carried, deposited)
	}
}

func TestBanker_DepositInsufficient(t *testing.T) {
	f := newBankerFixture(t)
	f.setWealth(t, 100, 0)

	runCmd(t, f.depositCmd(), f.alice, "5mk")
	if !strings.Contains(f.aOut.String(), "don't have") {
		t.Fatalf("expected insufficient-funds message, got %q", f.aOut.String())
	}
	carried, deposited := f.aliceWealth(t)
	if int64(carried) != 100 || int64(deposited) != 0 {
		t.Fatalf("balances mutated on failure: carried=%d deposited=%d", carried, deposited)
	}
}

func TestBanker_DepositRejectsZeroAndGarbage(t *testing.T) {
	f := newBankerFixture(t)
	f.setWealth(t, 1000, 0)

	for _, raw := range []string{"0", "0gc", "abc", "-5mk"} {
		f.aOut.Reset()
		runCmd(t, f.depositCmd(), f.alice, raw)
		if !strings.Contains(f.aOut.String(), "valid amount") {
			t.Fatalf("input %q: expected reject, got %q", raw, f.aOut.String())
		}
	}
}

func TestBanker_WithdrawSuccess(t *testing.T) {
	f := newBankerFixture(t)
	f.setWealth(t, 0, 1000)

	runCmd(t, f.withdrawCmd(), f.alice, "5mk")
	out := f.aOut.String()
	if !strings.Contains(out, "You withdraw") {
		t.Fatalf("missing withdraw echo; got %q", out)
	}
	carried, deposited := f.aliceWealth(t)
	if int64(carried) != 500 || int64(deposited) != 500 {
		t.Fatalf("balances wrong: carried=%d deposited=%d", carried, deposited)
	}
}

func TestBanker_WithdrawInsufficient(t *testing.T) {
	f := newBankerFixture(t)
	f.setWealth(t, 0, 100)

	runCmd(t, f.withdrawCmd(), f.alice, "5mk")
	if !strings.Contains(f.aOut.String(), "don't have") {
		t.Fatalf("expected insufficient-funds message, got %q", f.aOut.String())
	}
	carried, deposited := f.aliceWealth(t)
	if int64(carried) != 0 || int64(deposited) != 100 {
		t.Fatalf("balances mutated on failure: carried=%d deposited=%d", carried, deposited)
	}
}

func TestBanker_DepositAuditsOnSuccess(t *testing.T) {
	f := newBankerFixture(t)
	f.setWealth(t, 1000, 0)
	runCmd(t, f.depositCmd(), f.alice, "5mk")

	rows, err := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("recent audit: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.Verb == "deposit" && strings.Contains(r.Args, "amount=500cp") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no deposit audit row; rows=%+v", rows)
	}
}

func TestBanker_DepositRefusalDoesNotAudit(t *testing.T) {
	f := newBankerFixture(t)
	f.setWealth(t, 100, 0) // can't afford 5mk

	runCmd(t, f.depositCmd(), f.alice, "5mk")
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	for _, r := range rows {
		if r.Verb == "deposit" {
			t.Fatalf("refusal generated audit row: %+v", r)
		}
	}
}

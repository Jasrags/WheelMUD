package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// errNoBankerHere is the sentinel for "no banker in the current
// room". Callers translate it into a player-facing line.
var errNoBankerHere = errors.New("cmd: no banker here")

// resolvedBanker pairs a Banker config with the mob in the room that
// backs it. Mirrors resolvedShop.
type resolvedBanker struct {
	banker    repo.Banker
	keeper    creature.MobInstance
	keeperTpl creature.MobTemplate
}

func (r resolvedBanker) keeperName() string {
	if r.keeperTpl.Core.Name != "" {
		return r.keeperTpl.Core.Name
	}
	return "the banker"
}

// findBanker walks the mobs in roomID and returns the first one whose
// template has a matching bankers row. Returns errNoBankerHere if the
// room has no banker-capable mob.
func findBanker(ctx context.Context, roomID int64,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo, bankers repo.BankerRepo,
) (resolvedBanker, error) {
	if roomID == 0 {
		return resolvedBanker{}, errNoBankerHere
	}
	occupants, err := mobs.ListInRoom(ctx, roomID)
	if err != nil {
		return resolvedBanker{}, fmt.Errorf("list room mobs: %w", err)
	}
	for _, m := range occupants {
		b, err := bankers.GetByMobTemplateID(ctx, m.TemplateID)
		if errors.Is(err, repo.ErrBankerNotFound) {
			continue
		}
		if err != nil {
			return resolvedBanker{}, fmt.Errorf("get banker: %w", err)
		}
		tpl, err := templates.GetByID(ctx, m.TemplateID)
		if err != nil {
			return resolvedBanker{}, fmt.Errorf("get template: %w", err)
		}
		return resolvedBanker{banker: b, keeper: m, keeperTpl: tpl}, nil
	}
	return resolvedBanker{}, errNoBankerHere
}

// bankerOpen reports whether the banker is open right now per the
// clock. A nil HourSource is treated as always-open (test default).
func bankerOpen(b repo.Banker, clock HourSource) bool {
	if clock == nil {
		return true
	}
	return b.IsOpenAt(clock.HourOfDay())
}

// parseBankAmount parses the player-supplied amount and rejects zero
// / negative / unparseable values. Coin and bank balances are
// non-negative by invariant (currency.Amount.Sub returns
// ErrInsufficientFunds before going negative), so the verb layer is
// the right place to reject "0gc" etc.
func parseBankAmount(raw string) (currency.Amount, error) {
	amt, err := currency.Parse(raw)
	if err != nil {
		return 0, err
	}
	if int64(amt) <= 0 {
		return 0, errors.New("amount must be positive")
	}
	return amt, nil
}

// NewBalance builds the `balance` verb. Reports carried + on-deposit
// coin. Requires a banker in the room and that banker to be open —
// players can't read their balance from anywhere else (privacy + the
// banker is the in-fiction conduit).
func NewBalance(characters repo.CharacterRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	bankers repo.BankerRepo, clock HourSource,
) *telnet.Command {
	return &telnet.Command{
		Name: "balance",
		Help: "Balance — see how much coin you carry and have on deposit",
		Long: "Usage: balance\n\nMust be at a banker.",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			res, err := findBanker(c.Ctx, s.CurrentRoomID, mobs, templates, bankers)
			if errors.Is(err, errNoBankerHere) {
				return s.WriteString("{{There's no banker here.}}::yellow\r\n")
			}
			if err != nil {
				slog.Error("balance: banker lookup", "error", err)
				return s.WriteString("{{The bank is closed right now.}}::red\r\n")
			}
			if !bankerOpen(res.banker, clock) {
				return s.WriteString(fmt.Sprintf("{{%s isn't taking customers right now.}}::yellow\r\n", res.keeperName()))
			}
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("balance: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't find your records.}}::red\r\n")
			}
			return s.WriteString(fmt.Sprintf(
				"{{You carry %s. On deposit: %s.}}::cyan\r\n",
				char.Coin.Format(), char.BankBalance.Format()))
		},
	}
}

// NewDeposit builds the `deposit <amount>` verb. Moves coin from the
// character's purse into their bank balance via RecordCoin.
func NewDeposit(characters repo.CharacterRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	bankers repo.BankerRepo, clock HourSource, audits repo.AdminAuditRepo,
) *telnet.Command {
	return &telnet.Command{
		Name:    "deposit",
		Help:    "Deposit <amount> — put coin on deposit with the banker here",
		MinArgs: 1,
		Long:    "Usage: deposit <amount>\n\nAmount accepts forms like \"5gc\", \"2mk 3sp\", or a bare cp count.",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			res, err := findBanker(c.Ctx, s.CurrentRoomID, mobs, templates, bankers)
			if errors.Is(err, errNoBankerHere) {
				return s.WriteString("{{There's no banker here.}}::yellow\r\n")
			}
			if err != nil {
				slog.Error("deposit: banker lookup", "error", err)
				return s.WriteString("{{The bank is closed right now.}}::red\r\n")
			}
			if !bankerOpen(res.banker, clock) {
				return s.WriteString(fmt.Sprintf("{{%s isn't taking deposits right now.}}::yellow\r\n", res.keeperName()))
			}

			amt, err := parseBankAmount(c.Args[0])
			if err != nil {
				return s.WriteString("{{That isn't a valid amount.}}::yellow\r\n")
			}

			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("deposit: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't find your purse.}}::red\r\n")
			}
			newCoin, err := char.Coin.Sub(amt)
			if err != nil {
				if errors.Is(err, currency.ErrInsufficientFunds) {
					return s.WriteString(fmt.Sprintf("{{You don't have %s on you.}}::yellow\r\n", amt.Format()))
				}
				return s.WriteString("{{Something went wrong with the coin.}}::red\r\n")
			}
			newBank, err := char.BankBalance.Add(amt)
			if err != nil {
				return s.WriteString("{{Your account can't hold any more.}}::yellow\r\n")
			}

			if err := characters.RecordCoin(c.Ctx, char.ID, newCoin, newBank, char.CoinVersion); err != nil {
				if errors.Is(err, repo.ErrCoinConflict) {
					return s.WriteString("{{Your balance just changed — try again.}}::yellow\r\n")
				}
				slog.Error("deposit: record coin", "char", char.ID, "error", err)
				return s.WriteString("{{The banker fumbles the deposit.}}::red\r\n")
			}
			audit.Record(c.Ctx, audits, s, "deposit", res.keeperName(),
				fmt.Sprintf("amount=%dcp", int64(amt)))
			return s.WriteString(fmt.Sprintf(
				"{{You deposit %s. On deposit: %s.}}::cyan\r\n",
				amt.Format(), newBank.Format()))
		},
	}
}

// NewWithdraw builds the `withdraw <amount>` verb. Moves coin from
// bank balance back into the character's purse.
func NewWithdraw(characters repo.CharacterRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	bankers repo.BankerRepo, clock HourSource, audits repo.AdminAuditRepo,
) *telnet.Command {
	return &telnet.Command{
		Name:    "withdraw",
		Help:    "Withdraw <amount> — pull coin off deposit from the banker here",
		MinArgs: 1,
		Long:    "Usage: withdraw <amount>\n\nAmount accepts forms like \"5gc\", \"2mk 3sp\", or a bare cp count.",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			res, err := findBanker(c.Ctx, s.CurrentRoomID, mobs, templates, bankers)
			if errors.Is(err, errNoBankerHere) {
				return s.WriteString("{{There's no banker here.}}::yellow\r\n")
			}
			if err != nil {
				slog.Error("withdraw: banker lookup", "error", err)
				return s.WriteString("{{The bank is closed right now.}}::red\r\n")
			}
			if !bankerOpen(res.banker, clock) {
				return s.WriteString(fmt.Sprintf("{{%s isn't paying out right now.}}::yellow\r\n", res.keeperName()))
			}

			amt, err := parseBankAmount(c.Args[0])
			if err != nil {
				return s.WriteString("{{That isn't a valid amount.}}::yellow\r\n")
			}

			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("withdraw: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't find your records.}}::red\r\n")
			}
			newBank, err := char.BankBalance.Sub(amt)
			if err != nil {
				if errors.Is(err, currency.ErrInsufficientFunds) {
					return s.WriteString(fmt.Sprintf("{{You don't have %s on deposit.}}::yellow\r\n", amt.Format()))
				}
				return s.WriteString("{{Something went wrong with the coin.}}::red\r\n")
			}
			newCoin, err := char.Coin.Add(amt)
			if err != nil {
				return s.WriteString("{{You can't carry any more coin.}}::yellow\r\n")
			}

			if err := characters.RecordCoin(c.Ctx, char.ID, newCoin, newBank, char.CoinVersion); err != nil {
				if errors.Is(err, repo.ErrCoinConflict) {
					return s.WriteString("{{Your balance just changed — try again.}}::yellow\r\n")
				}
				slog.Error("withdraw: record coin", "char", char.ID, "error", err)
				return s.WriteString("{{The banker fumbles the withdrawal.}}::red\r\n")
			}
			audit.Record(c.Ctx, audits, s, "withdraw", res.keeperName(),
				fmt.Sprintf("amount=%dcp", int64(amt)))
			return s.WriteString(fmt.Sprintf(
				"{{You withdraw %s. You carry %s.}}::cyan\r\n",
				amt.Format(), newCoin.Format()))
		},
	}
}

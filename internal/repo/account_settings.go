package repo

// AccountSettings is the user-editable knob bag persisted in
// accounts.settings_json (migration 0035). The zero value is the
// "everything off / use server defaults" state — an account row that
// hasn't run through the §6 settings sub-menu yet round-trips through
// `{}` and produces this struct unchanged.
//
// Each field is consumed at a single application point (see
// docs in mode/postauth.go and mode/account_menu.go):
//   - ColorOverride   — promoteToGame replaces Session.ColorLevel
//   - PromptDefault   — CharacterCreate stamps onto Character.PromptTemplate
//   - WidthOverride   — promoteToGame clamps Session.Width
//   - Locale          — account-menu list formatter (LastPlayedAt)
//   - MOTDAlways      — postauth + AccountMenu.handleNews bypass last_news gate
type AccountSettings struct {
	// ColorOverride, when non-empty, replaces the TERM-detected
	// Session.ColorLevel after auth. Valid values: "none", "basic",
	// "16", "256", "truecolor". Empty = use TERM detection.
	ColorOverride string `json:"color,omitempty"`
	// PromptDefault is the cfmt prompt template forwarded to
	// CharacterCreate so newly-created characters inherit it. Existing
	// characters keep their own per-character prompt_template column.
	// Empty = use the server default (defaultPromptTemplate in main.go).
	PromptDefault string `json:"prompt,omitempty"`
	// WidthOverride, when > 0, clamps Session.Width after promote so a
	// terminal that lies via NAWS (or a client that doesn't send NAWS
	// at all) can be pinned to a sane wrap width. Validated 40..200
	// at the menu boundary; the repo persists whatever the menu
	// passed in, so a stale row with an out-of-range value loads as-is
	// and the apply path tolerates it.
	WidthOverride int `json:"width,omitempty"`
	// Locale is an IANA timezone string (e.g. "America/New_York")
	// applied to date display in the account menu's character list.
	// Empty = UTC. Validated via time.LoadLocation at the menu
	// boundary.
	Locale string `json:"locale,omitempty"`
	// MOTDAlways, when true, makes promoteToGame and the menu's
	// `news` verb call news.WriteMOTDBlock with a zero-time watermark
	// so every entry re-renders. Default (false) honours the
	// per-character last_news_seen gate.
	MOTDAlways bool `json:"motd_always,omitempty"`
}

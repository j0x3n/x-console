package protocol

import "time"

// Methods of B110 AI quotas. The agent announces CapQuota when this machine
// has Claude Code, Codex or Grok installed or signed in, and answers
// quota.read for them. DeepSeek balances are read by the server itself.
const (
	CapQuota        = "quota"
	MethodQuotaRead = "quota.read" // QuotaReadParams -> QuotaReading

	// CodeQuotaSignedOut: the account is not signed in or its sign-in
	// expired. The message says what to run on the machine.
	CodeQuotaSignedOut = "quota_signed_out"
	// CodeQuotaUnavailable: the vendor's endpoint or command failed, or what
	// it said could not be read.
	CodeQuotaUnavailable = "quota_unavailable"
)

// Kinds of account quota.read understands.
const (
	QuotaKindClaude = "claude"
	QuotaKindCodex  = "codex"
	QuotaKindGrok   = "grok"
)

// QuotaReadParams asks for one account's allowance. Home is the directory
// the account is signed in in (CLAUDE_CONFIG_DIR, CODEX_HOME, GROK_HOME); it
// must be absolute, or start with "~/". Empty means the CLI's own default.
type QuotaReadParams struct {
	Kind string `json:"kind"`
	Home string `json:"home,omitempty"`
}

// QuotaWindow is one allowance window of a subscription.
type QuotaWindow struct {
	Name        string     `json:"name"` // "5 小时", "7 天", "7 天 · Fable", "本月"
	UsedPercent float64    `json:"usedPercent"`
	ResetsAt    *time.Time `json:"resetsAt,omitempty"`
	SpanSecs    int64      `json:"spanSecs,omitempty"`
	// Model is the model a per-model window counts, empty for all models.
	Model string `json:"model,omitempty"`
	// Aside marks a window that does not stop the account when used up, such
	// as on-demand spending past the allowance.
	Aside bool `json:"aside,omitempty"`
}

// QuotaReading answers MethodQuotaRead. It never holds a token.
type QuotaReading struct {
	User    string        `json:"user,omitempty"`    // the account's email, when known
	Plan    string        `json:"plan,omitempty"`    // "plus", "pro"...
	Credits string        `json:"credits,omitempty"` // what is left of bought credits
	Windows []QuotaWindow `json:"windows"`
}

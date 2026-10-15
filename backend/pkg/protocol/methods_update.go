package protocol

// Agent self-update (B233). The panel carries the agent program that matches
// its own version and sends it to an agent whose version differs. The agent
// checks it, runs it once, swaps it in and restarts itself.
const (
	// CapUpdate is announced when the agent serves agent.update. An agent
	// whose config says "disableUpdate": true does not announce it.
	CapUpdate = "update"

	// MethodAgentUpdate is a stream: params AgentUpdateParams. The panel
	// sends the program in data frames (the stream has no half-close, so the
	// agent counts bytes: it reads exactly Size bytes). When the program is
	// written, matches SHA256, and `<program> version` prints Version, the
	// agent swaps it in and sends one empty data frame; then it restarts.
	// If anything is wrong the stream ends with an error whose message is
	// Chinese and fit to show to the user, and nothing is replaced.
	MethodAgentUpdate = "agent.update"
)

// AgentUpdateParams describes the program that follows in the stream.
type AgentUpdateParams struct {
	Version string `json:"version"` // what `<program> version` must print
	Size    int64  `json:"size"`    // bytes in the stream
	SHA256  string `json:"sha256"`  // hex digest of those bytes
}

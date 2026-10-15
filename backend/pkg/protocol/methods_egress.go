package protocol

// Egress (B208). The server's headless browser reaches the web through a
// connected agent: the server opens one net.dial stream per TCP connection
// and the agent connects to the target. The agent only connects to public
// addresses, so a page cannot use it to reach the agent's own network.
const (
	// CapEgress is announced when the agent serves net.dial. An agent whose
	// config says "disableEgress": true does not announce it.
	CapEgress = "egress"

	// MethodNetDial is a stream: params NetDialParams. The agent connects to
	// the target (10 second timeout). When the connection is made it sends one
	// empty data frame; if it fails the stream ends with an
	// error instead. After that every data frame carries raw TCP bytes in both
	// directions, and the stream ends when either side closes.
	MethodNetDial = "net.dial"
)

// NetDialParams is the target of one connection. Host is a name or an IP
// literal. The agent resolves names itself and checks the address it
// actually connects to.
type NetDialParams struct {
	Host string `json:"host"`
	Port int    `json:"port"` // 1 to 65535
}

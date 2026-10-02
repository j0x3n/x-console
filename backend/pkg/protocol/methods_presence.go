package protocol

const (
	MethodPresenceGet   = "presence.get"
	MethodNotifyShow    = "notify.show"
	EventPresenceUpdate = "presence.update"
	CapPresence         = "presence"
	CapNotifyShow       = "notify.show"
)

type PresenceSample struct {
	IdleSeconds int64 `json:"idleSeconds"`
	Locked      *bool `json:"locked,omitempty"`
	DisplayOff  *bool `json:"displayOff,omitempty"`
	Known       bool  `json:"known"`
}

type NotifyShowParams struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

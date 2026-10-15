package browser

// SetPace sets the percentage of the human delays of clicks and typing that
// this session really waits. 100 is the real pace.
func (x *Session) SetPace(percent int64) { x.paceScale.Store(percent) }

var (
	UserAgent    = userAgent
	LanguagesFor = languagesFor
)

package protocol

// B116: computer time statistics. A Windows agent reports one sample per
// minute: the program that had the foreground while the user was there.
const (
	CapScreenTime     = "screentime"
	EventScreenSample = "screentime.sample"
)

// ScreenSample is the foreground program of one minute.
type ScreenSample struct {
	// Minute is Unix seconds divided by 60.
	Minute int64 `json:"minute"`
	// App is the executable name, for example "Code.exe".
	App string `json:"app"`
	// Title is the window title. The server uses it to classify and does not
	// keep it unless the user turned that on.
	Title string `json:"title,omitempty"`
}

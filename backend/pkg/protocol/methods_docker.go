package protocol

import "time"

// Methods of M10 Docker. The agent announces CapDocker only when the Docker
// Engine socket answers. See docs/specs/M10.md.
const (
	MethodDockerPS     = "docker.ps"     // DockerPSParams -> DockerContainerList
	MethodDockerAction = "docker.action" // DockerActionParams -> nil
	MethodDockerStats  = "docker.stats"  // DockerStatsParams -> DockerStatsList
	MethodDockerImages = "docker.images" // nil -> DockerImageList

	// MethodDockerLogs is a stream. The agent sends log text (stdout and
	// stderr merged, Docker's multiplexing headers removed) and ends the
	// stream when the container's log ends. With Follow it keeps sending
	// until either side closes the stream.
	MethodDockerLogs = "docker.logs" // DockerLogsParams
)

// Container actions.
const (
	DockerStart   = "start"
	DockerStop    = "stop"
	DockerRestart = "restart"
	DockerRemove  = "remove"
)

// DockerPSParams lists containers. All includes stopped ones.
type DockerPSParams struct {
	All bool `json:"all,omitempty"`
}

// DockerPort is one published or exposed port.
type DockerPort struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort int    `json:"privatePort"`
	PublicPort  int    `json:"publicPort,omitempty"`
	Type        string `json:"type"` // tcp, udp
}

// DockerContainer is one container.
type DockerContainer struct {
	ID      string       `json:"id"` // full id
	Name    string       `json:"name"`
	Image   string       `json:"image"`
	State   string       `json:"state"`  // created, running, paused, restarting, removing, exited, dead
	Status  string       `json:"status"` // human text such as "Up 3 hours"
	Created time.Time    `json:"created"`
	Ports   []DockerPort `json:"ports"`
}

// DockerContainerList answers MethodDockerPS.
type DockerContainerList struct {
	Items []DockerContainer `json:"items"`
}

// DockerActionParams runs a container action. Remove forces removal of a
// running container.
type DockerActionParams struct {
	ID     string `json:"id"`
	Action string `json:"action"`
}

// DockerLogsParams starts a docker.logs stream. Tail defaults to 200 and is
// at most 5000.
type DockerLogsParams struct {
	ID         string `json:"id"`
	Tail       int    `json:"tail,omitempty"`
	Follow     bool   `json:"follow,omitempty"`
	Timestamps bool   `json:"timestamps,omitempty"`
}

// DockerStatsParams selects containers. Empty ID means every running one.
type DockerStatsParams struct {
	ID string `json:"id,omitempty"`
}

// DockerStats is one resource usage sample of a container.
type DockerStats struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpuPercent"` // percent of one core, like docker stats
	MemUsage   uint64  `json:"memUsage"`   // bytes, without page cache
	MemLimit   uint64  `json:"memLimit"`
	MemPercent float64 `json:"memPercent"`
	NetRx      uint64  `json:"netRx"` // bytes since start
	NetTx      uint64  `json:"netTx"`
	BlockRead  uint64  `json:"blockRead"`
	BlockWrite uint64  `json:"blockWrite"`
	PIDs       uint64  `json:"pids"`
}

// DockerStatsList answers MethodDockerStats.
type DockerStatsList struct {
	Items []DockerStats `json:"items"`
}

// DockerImage is one local image.
type DockerImage struct {
	ID         string    `json:"id"`
	Tags       []string  `json:"tags"`
	Size       int64     `json:"size"`
	Created    time.Time `json:"created"`
	Containers int       `json:"containers"` // -1 when Docker did not count
}

// DockerImageList answers MethodDockerImages.
type DockerImageList struct {
	Items []DockerImage `json:"items"`
}

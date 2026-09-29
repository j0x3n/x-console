package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// maxLogBytes caps a non-follow log read.
const maxLogBytes = 4 << 20

// dockerAgent resolves hostID to an online agent with the docker capability.
func (m *Module) dockerAgent(ctx context.Context, hostID string) (string, error) {
	if strings.HasPrefix(hostID, "ssh:") {
		return "", httpx.NewError(http.StatusNotImplemented, "unsupported", "SSH 主机不支持 Docker，请安装代理")
	}
	a, err := m.d.Agents.Get(ctx, hostID)
	if err != nil {
		return "", err
	}
	if !a.Online {
		return "", httpx.ErrAgentOffline
	}
	if !a.Has(protocol.CapDocker) {
		return "", httpx.NewError(http.StatusNotImplemented, "unsupported", "这台机器没有 Docker，或者代理读不到 Docker")
	}
	return a.ID, nil
}

// dockerErr gives agent errors the right status: an unknown container is a
// 404, a conflict a 409.
func dockerErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, rpc.ErrClosed) {
		return httpx.ErrAgentOffline
	}
	var he *httpx.Error
	if errors.As(err, &he) {
		switch he.Code {
		case "agent_" + protocol.CodeNotFound:
			return httpx.NewError(http.StatusNotFound, "not_found", "容器不存在: "+he.Message)
		case "agent_" + protocol.CodeExists:
			return httpx.NewError(http.StatusConflict, "conflict", he.Message)
		}
		return he
	}
	var pe *protocol.Error
	if errors.As(err, &pe) {
		switch pe.Code {
		case protocol.CodeNotFound:
			return httpx.NewError(http.StatusNotFound, "not_found", "容器不存在: "+pe.Message)
		case protocol.CodeUnsupported, protocol.CodeUnknownMethod:
			return httpx.NewError(http.StatusNotImplemented, "agent_"+pe.Code, pe.Message)
		}
		return httpx.NewError(http.StatusBadGateway, "agent_"+pe.Code, pe.Message)
	}
	return err
}

// dockerCall runs a docker.* request on the host's agent.
func (m *Module) dockerCall(ctx context.Context, hostID, method string, params, out any) error {
	id, err := m.dockerAgent(ctx, hostID)
	if err != nil {
		return err
	}
	return dockerErr(m.d.Agents.Call(ctx, id, method, params, out))
}

// ListContainers is GET /hosts/{hostId}/docker/containers.
func (m *Module) ListContainers(w http.ResponseWriter, r *http.Request, hostID string, params api.ListContainersParams) {
	var out protocol.DockerContainerList
	p := protocol.DockerPSParams{All: params.All != nil && *params.All}
	if err := m.dockerCall(r.Context(), hostID, protocol.MethodDockerPS, p, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.DockerContainer, 0, len(out.Items))
	for _, c := range out.Items {
		ports := make([]api.DockerPort, 0, len(c.Ports))
		for _, p := range c.Ports {
			ap := api.DockerPort{PrivatePort: p.PrivatePort, Type: p.Type}
			if p.IP != "" {
				ap.Ip = ptr(p.IP)
			}
			if p.PublicPort != 0 {
				ap.PublicPort = ptr(p.PublicPort)
			}
			ports = append(ports, ap)
		}
		items = append(items, api.DockerContainer{Id: c.ID, Name: c.Name, Image: c.Image, State: c.State, Status: c.Status,
			Created: c.Created, Ports: ports})
	}
	httpx.JSON(w, http.StatusOK, api.DockerContainerList{Items: items})
}

// containerNeedsElevation lists the actions that take a service away.
func containerNeedsElevation(action string) bool {
	return action == protocol.DockerStop || action == protocol.DockerRemove
}

// containerAction runs an action and audits it; callers check elevation.
func (m *Module) containerAction(ctx context.Context, hostID, container, action string) error {
	switch action {
	case protocol.DockerStart, protocol.DockerStop, protocol.DockerRestart, protocol.DockerRemove:
	default:
		return httpx.Invalid("action 只能是 start、stop、restart 或 remove")
	}
	if strings.TrimSpace(container) == "" {
		return httpx.Invalid("容器不能为空")
	}
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	err := m.dockerCall(cctx, hostID, protocol.MethodDockerAction, protocol.DockerActionParams{ID: container, Action: action}, nil)
	m.d.Audit.Record(ctx, "docker.container."+action, hostID, map[string]any{"container": container}, err)
	if err == nil {
		m.d.Bus.Publish("docker.container.changed", map[string]any{"hostId": hostID, "id": container, "action": action})
	}
	return err
}

// ContainerAction is POST /hosts/{hostId}/docker/containers/{containerId}/{action}.
func (m *Module) ContainerAction(w http.ResponseWriter, r *http.Request, hostID, container string, action api.DockerContainerAction) {
	if containerNeedsElevation(string(action)) {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if err := m.containerAction(r.Context(), hostID, container, string(action)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func logTail(tail *int) int {
	if tail == nil || *tail <= 0 {
		return 200
	}
	return min(*tail, 5000)
}

// openLogs starts a docker.logs stream. With p.Lines set, it asks the agent
// for lines with their stream only if the agent can; structured tells which
// it got.
func (m *Module) openLogs(ctx context.Context, hostID string, p protocol.DockerLogsParams) (s *rpc.Stream, structured bool, err error) {
	id, err := m.dockerAgent(ctx, hostID)
	if err != nil {
		return nil, false, err
	}
	if p.Lines {
		a, err := m.d.Agents.Get(ctx, id)
		if err != nil {
			return nil, false, err
		}
		p.Lines = a.Has(protocol.CapDockerLines)
	}
	s, err = m.d.Agents.Open(ctx, id, protocol.MethodDockerLogs, p)
	return s, p.Lines, dockerErr(err)
}

// GetContainerLogs is GET /hosts/{hostId}/docker/containers/{containerId}/logs.
func (m *Module) GetContainerLogs(w http.ResponseWriter, r *http.Request, hostID, container string, params api.GetContainerLogsParams) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	s, _, err := m.openLogs(ctx, hostID, protocol.DockerLogsParams{ID: container, Tail: logTail(params.Tail)})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer s.Close(nil)
	var buf []byte
	for len(buf) < maxLogBytes {
		b, err := s.Recv(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			httpx.Fail(w, r, dockerErr(err))
			return
		}
		buf = append(buf, b...)
	}
	httpx.JSON(w, http.StatusOK, api.DockerLogs{Lines: splitLines(string(buf))})
}

// splitLines splits log text and drops the empty line after a final newline.
func splitLines(s string) []string {
	s = strings.ToValidUTF8(strings.TrimRight(s, "\n"), "�")
	if s == "" {
		return []string{}
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// FollowContainerLogs is GET /hosts/{hostId}/docker/containers/{containerId}/logs/follow,
// a WebSocket. The host is checked before the upgrade so failures are plain
// JSON errors.
func (m *Module) FollowContainerLogs(w http.ResponseWriter, r *http.Request, hostID, container string, params api.FollowContainerLogsParams) {
	// The stream lives as long as the socket, not the request bookkeeping.
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()
	asJSON := params.Format != nil && *params.Format == api.Json
	s, structured, err := m.openLogs(ctx, hostID, protocol.DockerLogsParams{ID: container, Tail: logTail(params.Tail), Follow: true, Lines: asJSON})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer s.Close(nil)
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	// The browser only listens; CloseRead ends ctx when it goes away.
	ctx = ws.CloseRead(ctx)
	// An agent that cannot send lines sends text; make the text into lines
	// when the browser asked for JSON.
	var text *textLines
	if asJSON && !structured {
		text = &textLines{}
	}
	send := func(frames ...[]byte) error {
		for _, f := range frames {
			if err := ws.Write(ctx, websocket.MessageText, f); err != nil {
				return err
			}
		}
		return nil
	}
	for {
		b, err := s.Recv(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if text != nil {
					_ = send(text.finish()...)
				}
				_ = ws.Close(websocket.StatusNormalClosure, "log ended")
			} else if ctx.Err() == nil {
				_ = ws.Close(websocket.StatusInternalError, clip(errText(dockerErr(err)), 100))
			}
			return
		}
		frames := [][]byte{[]byte(strings.ToValidUTF8(string(b), "�"))}
		if text != nil {
			frames = text.add(string(b))
		}
		if err := send(frames...); err != nil {
			return
		}
	}
}

// maxFrameLines is the most lines the server puts in one JSON frame.
const maxFrameLines = 200

// textLines makes log text from an agent without CapDockerLines into frames
// of api.DockerLogLine. All lines count as standard output, because the
// agent does not tell.
type textLines struct{ rest string }

func (t *textLines) add(chunk string) [][]byte {
	t.rest += strings.ToValidUTF8(chunk, "�")
	i := strings.LastIndexByte(t.rest, '\n')
	if i < 0 {
		return nil
	}
	whole := t.rest[:i]
	t.rest = t.rest[i+1:]
	return packLines(strings.Split(whole, "\n"))
}

func (t *textLines) finish() [][]byte {
	rest := t.rest
	t.rest = ""
	if rest == "" {
		return nil
	}
	return packLines([]string{rest})
}

func packLines(lines []string) [][]byte {
	var out [][]byte
	for len(lines) > 0 {
		n := min(len(lines), maxFrameLines)
		batch := make([]protocol.DockerLogLine, 0, n)
		for _, l := range lines[:n] {
			batch = append(batch, protocol.DockerLogLine{Stream: "stdout", Text: strings.TrimSuffix(l, "\r")})
		}
		b, _ := json.Marshal(batch)
		out = append(out, b)
		lines = lines[n:]
	}
	return out
}

// GetDockerStats is GET /hosts/{hostId}/docker/stats.
func (m *Module) GetDockerStats(w http.ResponseWriter, r *http.Request, hostID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	var out protocol.DockerStatsList
	if err := m.dockerCall(ctx, hostID, protocol.MethodDockerStats, protocol.DockerStatsParams{}, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.DockerStats, 0, len(out.Items))
	for _, s := range out.Items {
		items = append(items, api.DockerStats{Id: s.ID, Name: s.Name, CpuPercent: s.CPUPercent, MemUsage: int64(s.MemUsage),
			MemLimit: int64(s.MemLimit), MemPercent: s.MemPercent, NetRx: int64(s.NetRx), NetTx: int64(s.NetTx),
			BlockRead: int64(s.BlockRead), BlockWrite: int64(s.BlockWrite), Pids: int64(s.PIDs)})
	}
	httpx.JSON(w, http.StatusOK, api.DockerStatsList{Items: items})
}

// ListImages is GET /hosts/{hostId}/docker/images.
func (m *Module) ListImages(w http.ResponseWriter, r *http.Request, hostID string) {
	var out protocol.DockerImageList
	if err := m.dockerCall(r.Context(), hostID, protocol.MethodDockerImages, nil, &out); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := make([]api.DockerImage, 0, len(out.Items))
	for _, x := range out.Items {
		tags := x.Tags
		if tags == nil {
			tags = []string{}
		}
		items = append(items, api.DockerImage{Id: x.ID, Tags: tags, Size: x.Size, Created: x.Created, Containers: x.Containers})
	}
	httpx.JSON(w, http.StatusOK, api.DockerImageList{Items: items})
}

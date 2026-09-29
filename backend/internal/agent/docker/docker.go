// Package docker serves the docker.* methods (M10) by talking to the Docker
// Engine API over its unix socket with plain net/http.
//
// The agent announces the docker capability only when the socket answers
// /_ping, so on machines without Docker (and on Windows, where Docker
// Desktop uses a named pipe) the feature stays hidden.
package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// DefaultSocket is where the Docker Engine listens on Linux.
const DefaultSocket = "/var/run/docker.sock"

const (
	callTimeout     = 30 * time.Second
	defaultLogTail  = 200
	maxLogTail      = 5000
	statsConcurrent = 8
	logFlushSize    = 32 << 10
)

// SocketPath returns the socket from DOCKER_HOST (unix://...) or the default.
func SocketPath() string {
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") {
		return strings.TrimPrefix(h, "unix://")
	}
	return DefaultSocket
}

// Client calls the Docker Engine API on one unix socket.
type Client struct {
	socket string
	http   *http.Client
}

// New builds a client for the socket at path.
func New(path string) *Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
		MaxIdleConns:    4,
		IdleConnTimeout: 30 * time.Second,
	}
	return &Client{socket: path, http: &http.Client{Transport: tr}}
}

// Available reports whether Docker answers on the default socket.
func Available() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return New(SocketPath()).Ping(ctx) == nil
}

// Register adds the docker.* methods using the default socket.
func Register(c *conn.Client) { RegisterClient(c, New(SocketPath())) }

// Registrar is the part of conn.Client the handlers need; tests pass an
// rpc peer wrapper.
type Registrar interface {
	Handle(method string, h rpc.Handler)
	HandleStream(method string, h rpc.StreamHandler)
}

// RegisterClient adds the docker.* methods backed by cl.
func RegisterClient(c Registrar, cl *Client) {
	c.Handle(protocol.MethodDockerPS, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.DockerPSParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return cl.PS(ctx, p)
	})
	c.Handle(protocol.MethodDockerAction, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.DockerActionParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, cl.Action(ctx, p)
	})
	c.Handle(protocol.MethodDockerStats, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.DockerStatsParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return cl.Stats(ctx, p)
	})
	c.Handle(protocol.MethodDockerImages, func(ctx context.Context, _ json.RawMessage) (any, error) {
		return cl.Images(ctx)
	})
	c.Handle(protocol.MethodDockerImageRemove, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.DockerImageRemoveParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, cl.RemoveImage(ctx, p.ID)
	})
	c.Handle(protocol.MethodDockerImagePrune, func(ctx context.Context, _ json.RawMessage) (any, error) {
		return cl.PruneImages(ctx)
	})
	c.HandleStream(protocol.MethodDockerLogs, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
		var p protocol.DockerLogsParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			select {
			case <-s.Context().Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		return cl.Logs(ctx, p, s)
	})
}

// ---- requests ----

// apiError is the JSON error body of the Engine API.
type apiError struct {
	Message string `json:"message"`
}

// do sends a request and returns the response for 2xx and 304. Other
// statuses become protocol errors with Docker's message.
func (c *Client) do(ctx context.Context, method, path string, q url.Values) (*http.Response, error) {
	u := "http://docker" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, rpcutil.BadParams("%v", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, rpcutil.Failed("docker is not reachable at %s: %v", c.socket, err)
	}
	if resp.StatusCode < 300 || resp.StatusCode == http.StatusNotModified {
		return resp, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var e apiError
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &e) == nil && e.Message != "" {
		msg = e.Message
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, &protocol.Error{Code: protocol.CodeNotFound, Message: msg}
	case http.StatusConflict:
		return nil, &protocol.Error{Code: protocol.CodeExists, Message: msg}
	case http.StatusBadRequest:
		return nil, &protocol.Error{Code: protocol.CodeBadParams, Message: msg}
	}
	return nil, rpcutil.Failed("docker: %s", msg)
}

// getJSON decodes a GET response into v.
func (c *Client) getJSON(ctx context.Context, path string, q url.Values, v any) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	resp, err := c.do(ctx, http.MethodGet, path, q)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return rpcutil.Failed("docker: bad response: %v", err)
	}
	return nil
}

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, callTimeout)
}

// Ping checks that the Engine answers.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodGet, "/_ping", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// checkID rejects ids that would change the request path.
func checkID(id string) error {
	if id == "" || strings.ContainsAny(id, "/?#") || strings.Contains(id, "..") {
		return rpcutil.BadParams("invalid container id %q", id)
	}
	return nil
}

// ---- containers ----

type psItem struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
	Ports   []struct {
		IP          string `json:"IP"`
		PrivatePort int    `json:"PrivatePort"`
		PublicPort  int    `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
}

// PS lists containers.
func (c *Client) PS(ctx context.Context, p protocol.DockerPSParams) (protocol.DockerContainerList, error) {
	q := url.Values{}
	if p.All {
		q.Set("all", "1")
	}
	var items []psItem
	if err := c.getJSON(ctx, "/containers/json", q, &items); err != nil {
		return protocol.DockerContainerList{}, err
	}
	out := protocol.DockerContainerList{Items: make([]protocol.DockerContainer, 0, len(items))}
	for _, x := range items {
		ct := protocol.DockerContainer{ID: x.ID, Name: containerName(x.Names), Image: x.Image, State: x.State,
			Status: x.Status, Created: time.Unix(x.Created, 0).UTC(), Ports: []protocol.DockerPort{}}
		for _, pt := range x.Ports {
			ct.Ports = append(ct.Ports, protocol.DockerPort{IP: pt.IP, PrivatePort: pt.PrivatePort, PublicPort: pt.PublicPort, Type: pt.Type})
		}
		out.Items = append(out.Items, ct)
	}
	return out, nil
}

func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

// Action starts, stops, restarts or removes a container.
func (c *Client) Action(ctx context.Context, p protocol.DockerActionParams) error {
	if err := checkID(p.ID); err != nil {
		return err
	}
	id := url.PathEscape(p.ID)
	// Stopping waits up to 10 seconds for the container before killing it.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var (
		resp *http.Response
		err  error
	)
	switch p.Action {
	case protocol.DockerStart, protocol.DockerStop, protocol.DockerRestart:
		resp, err = c.do(ctx, http.MethodPost, "/containers/"+id+"/"+p.Action, nil)
	case protocol.DockerRemove:
		resp, err = c.do(ctx, http.MethodDelete, "/containers/"+id, url.Values{"force": {"1"}})
	default:
		return rpcutil.BadParams("unknown action %q", p.Action)
	}
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ---- logs ----

type inspect struct {
	Config struct {
		Tty bool `json:"Tty"`
	} `json:"Config"`
}

// Logs writes the container log to w. Without a TTY Docker multiplexes
// stdout and stderr with 8-byte frame headers; they are removed here.
func (c *Client) Logs(ctx context.Context, p protocol.DockerLogsParams, w io.Writer) error {
	if err := checkID(p.ID); err != nil {
		return err
	}
	var info inspect
	if err := c.getJSON(ctx, "/containers/"+url.PathEscape(p.ID)+"/json", nil, &info); err != nil {
		return err
	}
	tail := p.Tail
	if tail <= 0 {
		tail = defaultLogTail
	}
	tail = min(tail, maxLogTail)
	q := url.Values{"stdout": {"1"}, "stderr": {"1"}, "tail": {strconv.Itoa(tail)}}
	if p.Follow {
		q.Set("follow", "1")
	}
	if p.Timestamps || p.Lines {
		q.Set("timestamps", "1")
	}
	resp, err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(p.ID)+"/logs", q)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Close the body when ctx ends so a blocked follow read returns.
	stop := context.AfterFunc(ctx, func() { resp.Body.Close() })
	defer stop()
	if p.Lines {
		err = logLines(resp.Body, w, info.Config.Tty)
	} else if info.Config.Tty {
		err = copyRaw(resp.Body, w)
	} else {
		err = Demux(resp.Body, w)
	}
	if ctx.Err() != nil {
		return nil // the reader went away; not an error for a follow stream
	}
	return err
}

// copyRaw copies TTY output in chunks as they arrive.
func copyRaw(r io.Reader, w io.Writer) error {
	buf := make([]byte, logFlushSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Demux reads Docker's multiplexed log format from r and writes the
// payloads to w. Output is batched while more data is already buffered, so
// a long tail becomes a few large writes and a follow stream still flushes
// every line as soon as it arrives.
func Demux(r io.Reader, w io.Writer) error {
	br := bufio.NewReaderSize(r, 64<<10)
	var out []byte
	flush := func() error {
		if len(out) == 0 {
			return nil
		}
		_, err := w.Write(out)
		out = out[:0]
		return err
	}
	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(br, hdr); err != nil {
			if ferr := flush(); ferr != nil {
				return ferr
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		size := int(binary.BigEndian.Uint32(hdr[4:]))
		start := len(out)
		out = append(out, make([]byte, size)...)
		if _, err := io.ReadFull(br, out[start:]); err != nil {
			out = out[:start]
			if ferr := flush(); ferr != nil {
				return ferr
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		if len(out) >= logFlushSize || br.Buffered() == 0 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
}

// ---- stats ----

// rawStats is the part of /containers/{id}/stats we use.
type rawStats struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PidsStats struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
	Networks    map[string]netStats `json:"networks"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	CPUStats    cpuStats `json:"cpu_stats"`
	PreCPUStats cpuStats `json:"precpu_stats"`
}

type netStats struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

type cpuStats struct {
	CPUUsage struct {
		TotalUsage  uint64   `json:"total_usage"`
		PercpuUsage []uint64 `json:"percpu_usage"`
	} `json:"cpu_usage"`
	SystemUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs  uint32 `json:"online_cpus"`
}

// computeStats turns a raw sample into the numbers `docker stats` shows.
func computeStats(r rawStats) protocol.DockerStats {
	s := protocol.DockerStats{ID: r.ID, Name: strings.TrimPrefix(r.Name, "/"), PIDs: r.PidsStats.Current,
		MemLimit: r.MemoryStats.Limit}
	cpuDelta := float64(r.CPUStats.CPUUsage.TotalUsage) - float64(r.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(r.CPUStats.SystemUsage) - float64(r.PreCPUStats.SystemUsage)
	cpus := float64(r.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(r.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpuDelta > 0 && sysDelta > 0 && cpus > 0 {
		s.CPUPercent = round2(cpuDelta / sysDelta * cpus * 100)
	}
	// Like the docker CLI: cgroup v1 reports total_inactive_file, v2
	// inactive_file; both are page cache that can be dropped.
	used := r.MemoryStats.Usage
	inactive, ok := r.MemoryStats.Stats["total_inactive_file"]
	if !ok {
		inactive = r.MemoryStats.Stats["inactive_file"]
	}
	if inactive < used {
		used -= inactive
	}
	s.MemUsage = used
	if s.MemLimit > 0 {
		s.MemPercent = round2(float64(used) / float64(s.MemLimit) * 100)
	}
	for _, n := range r.Networks {
		s.NetRx += n.RxBytes
		s.NetTx += n.TxBytes
	}
	for _, b := range r.BlkioStats.IOServiceBytesRecursive {
		switch strings.ToLower(b.Op) {
		case "read":
			s.BlockRead += b.Value
		case "write":
			s.BlockWrite += b.Value
		}
	}
	return s
}

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

// Stats samples one container, or every running container when p.ID is
// empty. Docker takes about a second per sample; they run in parallel.
func (c *Client) Stats(ctx context.Context, p protocol.DockerStatsParams) (protocol.DockerStatsList, error) {
	var ids []string
	names := map[string]string{}
	if p.ID != "" {
		if err := checkID(p.ID); err != nil {
			return protocol.DockerStatsList{}, err
		}
		ids = []string{p.ID}
	} else {
		list, err := c.PS(ctx, protocol.DockerPSParams{})
		if err != nil {
			return protocol.DockerStatsList{}, err
		}
		for _, x := range list.Items {
			ids = append(ids, x.ID)
			names[x.ID] = x.Name
		}
	}
	out := make([]protocol.DockerStats, len(ids))
	errs := make([]error, len(ids))
	sem := make(chan struct{}, statsConcurrent)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var raw rawStats
			errs[i] = c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/stats", url.Values{"stream": {"false"}}, &raw)
			out[i] = computeStats(raw)
			if out[i].ID == "" {
				out[i].ID = id
			}
			if out[i].Name == "" {
				out[i].Name = names[id]
			}
		}()
	}
	wg.Wait()
	res := protocol.DockerStatsList{Items: make([]protocol.DockerStats, 0, len(ids))}
	for i := range ids {
		if errs[i] != nil {
			// A container that stopped in the meantime is skipped; a failure
			// on a single requested container is reported.
			if p.ID != "" {
				return protocol.DockerStatsList{}, errs[i]
			}
			continue
		}
		res.Items = append(res.Items, out[i])
	}
	return res, nil
}

// ---- images ----

type imageItem struct {
	ID         string   `json:"Id"`
	RepoTags   []string `json:"RepoTags"`
	Size       int64    `json:"Size"`
	Created    int64    `json:"Created"`
	Containers int      `json:"Containers"`
}

// Images lists local images.
func (c *Client) Images(ctx context.Context) (protocol.DockerImageList, error) {
	var items []imageItem
	if err := c.getJSON(ctx, "/images/json", nil, &items); err != nil {
		return protocol.DockerImageList{}, err
	}
	out := protocol.DockerImageList{Items: make([]protocol.DockerImage, 0, len(items))}
	for _, x := range items {
		tags := []string{}
		for _, t := range x.RepoTags {
			if t != "<none>:<none>" {
				tags = append(tags, t)
			}
		}
		out.Items = append(out.Items, protocol.DockerImage{ID: x.ID, Tags: tags, Size: x.Size,
			Created: time.Unix(x.Created, 0).UTC(), Containers: x.Containers})
	}
	return out, nil
}

// ---- image cleanup ----

// checkImageRef allows an image ID or a tag such as "library/nginx:1.27".
func checkImageRef(ref string) error {
	if ref == "" || len(ref) > 300 || strings.Contains(ref, "..") {
		return rpcutil.BadParams("invalid image %q", ref)
	}
	for _, r := range ref {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(":._/@-", r)
		if !ok {
			return rpcutil.BadParams("invalid image %q", ref)
		}
	}
	return nil
}

// RemoveImage deletes an image without force. Docker answers 409 when a
// container still uses it, which comes back as protocol.CodeExists.
func (c *Client) RemoveImage(ctx context.Context, ref string) error {
	if err := checkImageRef(ref); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	resp, err := c.do(ctx, http.MethodDelete, "/images/"+strings.ReplaceAll(url.PathEscape(ref), "%2F", "/"), nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// PruneImages deletes every image no container uses, tagged or not.
func (c *Client) PruneImages(ctx context.Context) (protocol.DockerImagePruneResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	resp, err := c.do(ctx, http.MethodPost, "/images/prune", url.Values{"filters": {`{"dangling":["false"]}`}})
	if err != nil {
		return protocol.DockerImagePruneResult{}, err
	}
	defer resp.Body.Close()
	var out struct {
		ImagesDeleted []struct {
			Deleted string `json:"Deleted"`
		} `json:"ImagesDeleted"`
		SpaceReclaimed int64 `json:"SpaceReclaimed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return protocol.DockerImagePruneResult{}, rpcutil.Failed("docker: bad prune answer: %v", err)
	}
	res := protocol.DockerImagePruneResult{SpaceReclaimed: out.SpaceReclaimed}
	for _, x := range out.ImagesDeleted {
		if x.Deleted != "" {
			res.Deleted++
		}
	}
	return res, nil
}

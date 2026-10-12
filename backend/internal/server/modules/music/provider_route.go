package music

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/url"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const sourceAgentTTL = 10 * time.Minute

type sourceStatusError struct {
	status  int
	message string
}

func (e sourceStatusError) Error() string {
	if e.message != "" {
		return e.message
	}
	return fmt.Sprintf("HTTP %d", e.status)
}

type sourceFormatError struct{ message string }

func (e sourceFormatError) Error() string { return e.message }
func (e sourceFormatError) Unwrap() error { return errSourceFormat }

func retrySource(err error) bool {
	if errors.Is(err, errSourceFormat) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var status sourceStatusError
	if errors.As(err, &status) {
		return status.status == 403 || status.status == 451 || status.status == 429 || status.status >= 500 && status.status <= 599
	}
	var network *url.Error
	return errors.As(err, &network)
}

type sourceRouteKey struct{}
type sourceRoute struct {
	forceDirect bool // diagnostics bypass the preference cache
	agent       *agenthub.Agent
	useAgent    bool
	usedAgent   bool
	directError error
}

func (s *providerSet) bindAgents(hub *agenthub.Hub) {
	s.agents = hub
	hub.OnEvent(protocol.EventMetrics, func(id string, _ json.RawMessage) {
		s.routeMu.Lock()
		s.heartbeats[id] = s.now()
		s.routeMu.Unlock()
	})
}

func (s *providerSet) fetchAgent(ctx context.Context) *agenthub.Agent {
	if s.agents == nil {
		return nil
	}
	agents, err := s.agents.List(ctx)
	if err != nil {
		return nil
	}
	s.routeMu.Lock()
	defer s.routeMu.Unlock()
	var best *agenthub.Agent
	var latest time.Time
	for _, a := range agents {
		if !a.Online || !strings.EqualFold(a.Os, "windows") || !a.Has(protocol.CapNetFetch) {
			continue
		}
		seen := a.CreatedAt
		if a.LastSeenAt != nil {
			seen = *a.LastSeenAt
		}
		if beat := s.heartbeats[a.ID]; beat.After(seen) {
			seen = beat
		}
		if best == nil || seen.After(latest) || seen.Equal(latest) && a.ID > best.ID {
			copy := a
			best = &copy
			latest = seen
		}
	}
	return best
}

type routedProvider struct {
	s *providerSet
	provider
}

func sourceCall[T any](ctx context.Context, p routedProvider, fn func(context.Context) (T, error)) (T, error) {
	route, _ := ctx.Value(sourceRouteKey{}).(*sourceRoute)
	if route == nil {
		route = &sourceRoute{}
	}
	ctx = context.WithValue(ctx, sourceRouteKey{}, route)
	p.s.routeMu.Lock()
	preferred := p.s.now().Before(p.s.preferAgent[p.Name()])
	p.s.routeMu.Unlock()
	if preferred && !route.forceDirect {
		route.agent = p.s.fetchAgent(ctx)
		route.useAgent = route.agent != nil
	}
	out, err := fn(ctx)
	if err != nil && ctx.Err() == nil && retrySource(err) && !route.usedAgent {
		if agent := p.s.fetchAgent(ctx); agent != nil {
			route.directError = err
			route.agent = agent
			route.useAgent = true
			out, err = fn(ctx)
		}
	}
	p.s.routeMu.Lock()
	if err == nil && route.usedAgent && route.directError != nil {
		p.s.preferAgent[p.Name()] = p.s.now().Add(sourceAgentTTL)
	}
	if err != nil && route.usedAgent {
		delete(p.s.preferAgent, p.Name())
	}
	p.s.routeMu.Unlock()
	return out, err
}

func (p routedProvider) Search(ctx context.Context, q query) ([]candidate, error) {
	return sourceCall(ctx, p, func(ctx context.Context) ([]candidate, error) { return p.provider.Search(ctx, q) })
}
func (p routedProvider) FetchLyrics(ctx context.Context, c candidate) (string, error) {
	return sourceCall(ctx, p, func(ctx context.Context) (string, error) { return p.provider.FetchLyrics(ctx, c) })
}
func (p routedProvider) FetchCover(ctx context.Context, c candidate) ([]byte, error) {
	return sourceCall(ctx, p, func(ctx context.Context) ([]byte, error) {
		data, err := p.provider.FetchCover(ctx, c)
		if err == nil {
			if _, _, decodeErr := image.DecodeConfig(bytes.NewReader(data)); decodeErr != nil {
				return nil, errSourceFormat
			}
		}
		return data, err
	})
}

// do is shared by searches, lyrics and cover downloads.
func (s *providerSet) do(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	route, _ := ctx.Value(sourceRouteKey{}).(*sourceRoute)
	if route == nil {
		return s.direct(ctx, rawURL, headers)
	}
	if route.useAgent {
		return s.agentGET(ctx, route, rawURL, headers)
	}
	body, err := s.direct(ctx, rawURL, headers)
	if err == nil || ctx.Err() != nil || !retrySource(err) {
		return body, err
	}
	agent := s.fetchAgent(ctx)
	if agent == nil || protocol.ValidateNetFetchURL(rawURL) != nil {
		return body, err
	}
	route.directError = err
	route.agent = agent
	route.useAgent = true
	return s.agentGET(ctx, route, rawURL, headers)
}

func (s *providerSet) agentGET(ctx context.Context, route *sourceRoute, rawURL string, headers map[string]string) ([]byte, error) {
	p := protocol.NetFetchParams{URL: rawURL, Headers: map[string]string{"User-Agent": userAgent}, MaxBytes: maxBody, TimeoutMs: protocol.NetFetchDefaultTimeoutMs}
	for k, v := range headers {
		if protocol.NetFetchHeaderAllowed(k) {
			p.Headers[k] = v
		}
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	route.usedAgent = true
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutMs)*time.Millisecond+time.Second)
	defer cancel()
	var out protocol.NetFetchResult
	if err := s.agents.Call(ctx, route.agent.ID, protocol.MethodNetFetch, p, &out); err != nil {
		return nil, err
	}
	if out.Status != 200 {
		return nil, sourceStatusError{status: out.Status}
	}
	if len(out.Body) > maxBody {
		return nil, errors.New("音乐来源响应超过大小限制")
	}
	return out.Body, nil
}

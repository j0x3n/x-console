package protocol

import (
	"errors"
	"slices"
)

// Network proxy (M9). An agent on the home network forwards HTTP requests
// and WebSocket connections to services the server cannot reach, such as a
// Home Assistant without a public address. The agent only connects to
// loopback and private-network addresses; it is not an open proxy.
const (
	CapProxy = "proxy"

	// MethodHTTPProxy is a request: HTTPProxyParams -> HTTPProxyResult.
	// Redirects are not followed.
	MethodHTTPProxy = "http.proxy"
	// MethodWSProxy is a stream: params WSProxyParams. Data frames in both
	// directions carry WebSocket text messages, framed as described at
	// WSProxyChunk. The stream ends when either side closes.
	MethodWSProxy = "ws.proxy"
)

// CodeForbiddenTarget is returned when the target is not on a private network.
const CodeForbiddenTarget = "forbidden_target"

// HTTPProxyParams describes one HTTP request made by the agent.
type HTTPProxyParams struct {
	Method string              `json:"method"`
	URL    string              `json:"url"` // http:// or https://
	Header map[string][]string `json:"header,omitempty"`
	Body   []byte              `json:"body,omitempty"`
	// TimeoutSeconds defaults to 15 and is capped at 60.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
}

// HTTPProxyResult is the response. Bodies over HTTPProxyMaxBody fail.
type HTTPProxyResult struct {
	Status int                 `json:"status"`
	Header map[string][]string `json:"header,omitempty"`
	Body   []byte              `json:"body,omitempty"`
}

// HTTPProxyMaxBody keeps a response well inside one protocol frame.
const HTTPProxyMaxBody = 8 << 20

// WSProxyParams opens a WebSocket from the agent.
type WSProxyParams struct {
	URL    string              `json:"url"` // ws:// or wss://
	Header map[string][]string `json:"header,omitempty"`
}

// A WebSocket message can be larger than one protocol frame (Home
// Assistant's get_states often is), so each message is sent as one or more
// data frames of at most WSProxyChunk payload bytes. Every frame starts with
// a flag byte: WSProxyMore when more frames of the same message follow,
// WSProxyFinal on the last one.
const (
	WSProxyChunk      = 256 << 10
	WSProxyMaxMessage = 64 << 20

	WSProxyMore  byte = 0
	WSProxyFinal byte = 1
)

// WSProxySplit frames one message.
func WSProxySplit(msg []byte) [][]byte {
	var out [][]byte
	for {
		n := min(len(msg), WSProxyChunk)
		flag := WSProxyMore
		if n == len(msg) {
			flag = WSProxyFinal
		}
		out = append(out, append([]byte{flag}, msg[:n]...))
		msg = msg[n:]
		if flag == WSProxyFinal {
			return out
		}
	}
}

// WSProxyJoiner reassembles messages from frames.
type WSProxyJoiner struct{ buf []byte }

// Add takes one frame and returns the message when it is complete.
func (j *WSProxyJoiner) Add(frame []byte) (msg []byte, done bool, err error) {
	if len(frame) == 0 || frame[0] > WSProxyFinal {
		return nil, false, errors.New("ws.proxy: bad frame")
	}
	if len(j.buf)+len(frame)-1 > WSProxyMaxMessage {
		return nil, false, errors.New("ws.proxy: message too large")
	}
	j.buf = append(j.buf, frame[1:]...)
	if frame[0] == WSProxyMore {
		return nil, false, nil
	}
	msg = slices.Clone(j.buf)
	j.buf = j.buf[:0]
	return msg, true, nil
}

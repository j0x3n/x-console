package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

// Frame is one picture of the page for the takeover view (B205).
type Frame struct {
	JPEG []byte
	// Width and Height are the page size in CSS pixels that the picture
	// shows. Input coordinates use the same space.
	Width, Height float64
}

// Screencast sends pictures of the page to fn until ctx ends or the session
// closes. Chrome only sends a new picture when the page changes.
func (x *Session) Screencast(ctx context.Context, fn func(Frame)) error {
	if err := x.alive(); err != nil {
		return err
	}
	ch, cancel := x.c.subscribe(x.sid, "Page.screencastFrame", 4)
	defer cancel()
	if err := x.c.call(ctx, x.sid, "Page.startScreencast", map[string]any{"format": "jpeg", "quality": 60,
		"maxWidth": ViewWidth, "maxHeight": ViewHeight, "everyNthFrame": 1}, nil); err != nil {
		return err
	}
	defer func() {
		sctx, scancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer scancel()
		_ = x.c.call(sctx, x.sid, "Page.stopScreencast", nil, nil)
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-x.done:
			return nil
		case ev := <-ch:
			var p struct {
				Data      string `json:"data"`
				SessionID int    `json:"sessionId"`
				Metadata  struct {
					DeviceWidth  float64 `json:"deviceWidth"`
					DeviceHeight float64 `json:"deviceHeight"`
				} `json:"metadata"`
			}
			if json.Unmarshal(ev.Params, &p) != nil {
				continue
			}
			go func() {
				_ = x.c.call(context.Background(), x.sid, "Page.screencastFrameAck", map[string]any{"sessionId": p.SessionID}, nil)
			}()
			img, err := base64.StdEncoding.DecodeString(p.Data)
			if err != nil {
				continue
			}
			fn(Frame{JPEG: img, Width: p.Metadata.DeviceWidth, Height: p.Metadata.DeviceHeight})
		}
	}
}

// Input is one event from the takeover view.
type Input struct {
	// Type is "down", "move", "drag" (move with the button held), "up",
	// "click", "wheel", "text" or "key".
	Type   string  `json:"type"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	DeltaY float64 `json:"deltaY"`
	Text   string  `json:"text"`
	Key    string  `json:"key"`
}

// Send replays a mouse or keyboard event from the user on the page. The
// page may leave the allowed hosts this way; Check after the user is done.
func (x *Session) Send(ctx context.Context, in Input) error {
	if err := x.alive(); err != nil {
		return err
	}
	mouse := func(typ, button string, clicks int) error {
		p := map[string]any{"type": typ, "x": in.X, "y": in.Y, "button": button, "clickCount": clicks}
		if button == "left" {
			p["buttons"] = 1
		}
		return x.c.call(ctx, x.sid, "Input.dispatchMouseEvent", p, nil)
	}
	switch in.Type {
	case "down":
		return mouse("mousePressed", "left", 1)
	case "move":
		return mouse("mouseMoved", "none", 0)
	case "drag":
		return mouse("mouseMoved", "left", 0)
	case "up":
		return mouse("mouseReleased", "left", 1)
	case "click":
		return x.clickAt(ctx, in.X, in.Y)
	case "wheel":
		return x.c.call(ctx, x.sid, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": in.X, "y": in.Y, "deltaX": 0, "deltaY": in.DeltaY}, nil)
	case "text":
		if len(in.Text) > 2000 {
			return errors.New("一次输入的文字太长")
		}
		return x.c.call(ctx, x.sid, "Input.insertText", map[string]any{"text": in.Text}, nil)
	case "key":
		return x.press(ctx, in.Key)
	}
	return errors.New("不认识的输入")
}

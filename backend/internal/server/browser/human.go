package browser

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
	"unicode/utf8"
)

// The AI clicks and types at the speed of a person (B226), because a click
// that appears at the target without any mouse movement and text that appears
// all at once are easy to tell from a person. What the user does in the
// takeover view is sent as it comes and does not go through here.

// maxHumanType is the longest text that is typed one character at a time.
// Longer text is inserted at once, or it would take too long.
const maxHumanType = 200

// pause waits for a random time between lo and hi, scaled by paceScale.
func (x *Session) pause(ctx context.Context, lo, hi time.Duration) error {
	d := lo + time.Duration(rand.Int64N(int64(hi-lo)+1))
	// Tests lower the pace of a session to keep themselves fast.
	if p := x.paceScale.Load(); p != 0 {
		d = d * time.Duration(p) / 100
	}
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-x.done:
		return x.alive()
	}
}

func (x *Session) setMouse(px, py float64) {
	x.mouseMu.Lock()
	x.mouseX, x.mouseY, x.mouseSet = px, py, true
	x.mouseMu.Unlock()
}

// mouse returns the last position of the mouse. A session that has not used
// the mouse yet starts somewhere in the upper middle of the page.
func (x *Session) mouse() (float64, float64) {
	x.mouseMu.Lock()
	defer x.mouseMu.Unlock()
	if !x.mouseSet {
		x.mouseX = ViewWidth * (0.3 + 0.4*rand.Float64())
		x.mouseY = ViewHeight * (0.1 + 0.3*rand.Float64())
		x.mouseSet = true
	}
	return x.mouseX, x.mouseY
}

// mouseEvent sends one raw mouse event and remembers where the mouse is.
func (x *Session) mouseEvent(ctx context.Context, typ string, px, py float64) error {
	p := map[string]any{"type": typ, "x": px, "y": py, "button": "left", "clickCount": 1}
	if typ == "mouseMoved" {
		p["button"] = "none"
		p["clickCount"] = 0
	}
	x.setMouse(px, py)
	return x.c.call(ctx, x.sid, "Input.dispatchMouseEvent", p, nil)
}

// clickAt presses and releases the left button at once. The takeover view
// uses it.
func (x *Session) clickAt(ctx context.Context, px, py float64) error {
	for _, typ := range []string{"mouseMoved", "mousePressed", "mouseReleased"} {
		if err := x.mouseEvent(ctx, typ, px, py); err != nil {
			return err
		}
	}
	return nil
}

// humanClick moves the mouse from where it was to a point near the middle of
// the element (a box of w by h around cx, cy), then presses and releases the
// button a little later.
func (x *Session) humanClick(ctx context.Context, cx, cy, w, h float64) error {
	// A few pixels off the middle, never outside the element.
	tx := cx + (rand.Float64()*2-1)*math.Min(w*0.2, 8)
	ty := cy + (rand.Float64()*2-1)*math.Min(h*0.2, 8)
	if err := x.moveMouse(ctx, tx, ty); err != nil {
		return err
	}
	if err := x.mouseEvent(ctx, "mousePressed", tx, ty); err != nil {
		return err
	}
	if err := x.pause(ctx, 50*time.Millisecond, 120*time.Millisecond); err != nil {
		return err
	}
	return x.mouseEvent(ctx, "mouseReleased", tx, ty)
}

// moveMouse moves the mouse to tx, ty in 5 to 9 steps along a slight curve,
// 10 to 30 milliseconds apart, and ends with the pause after the last step.
func (x *Session) moveMouse(ctx context.Context, tx, ty float64) error {
	fx, fy := x.mouse()
	steps := 5 + rand.IntN(5)
	dist := math.Hypot(tx-fx, ty-fy)
	// The path bends to one side by a few percent of the distance.
	bow := (rand.Float64()*2 - 1) * math.Min(dist*0.06, 20)
	nx, ny := 0.0, 0.0
	if dist > 0 {
		nx, ny = -(ty-fy)/dist, (tx-fx)/dist
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		e := t * t * (3 - 2*t)
		px := fx + (tx-fx)*e + nx*bow*math.Sin(math.Pi*t)
		py := fy + (ty-fy)*e + ny*bow*math.Sin(math.Pi*t)
		if i == steps {
			px, py = tx, ty
		}
		if err := x.mouseEvent(ctx, "mouseMoved", math.Max(px, 0), math.Max(py, 0)); err != nil {
			return err
		}
		if err := x.pause(ctx, 10*time.Millisecond, 30*time.Millisecond); err != nil {
			return err
		}
	}
	return nil
}

// typeText types text into the focused element. Up to maxHumanType
// characters go one at a time, 30 to 120 milliseconds apart.
func (x *Session) typeText(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) > maxHumanType {
		return x.insertText(ctx, text)
	}
	first := true
	for _, r := range text {
		if !first {
			if err := x.pause(ctx, 30*time.Millisecond, 120*time.Millisecond); err != nil {
				return err
			}
		}
		first = false
		if err := x.typeRune(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (x *Session) insertText(ctx context.Context, text string) error {
	return x.c.call(ctx, x.sid, "Input.insertText", map[string]any{"text": text}, nil)
}

// typeRune presses the key of one character. The page gets keydown, keypress,
// input and keyup like from a keyboard, also for characters that have no key
// on it, such as Chinese. Control characters such as a line break are
// inserted as text, because the Enter key would submit a form.
func (x *Session) typeRune(ctx context.Context, r rune) error {
	if r < 0x20 || r == 0x7f {
		return x.insertText(ctx, string(r))
	}
	s := string(r)
	down := map[string]any{"type": "keyDown", "text": s, "unmodifiedText": s, "key": s}
	up := map[string]any{"type": "keyUp", "key": s}
	switch {
	case r >= 'a' && r <= 'z':
		down["code"], up["code"] = "Key"+string(r-32), "Key"+string(r-32)
		down["windowsVirtualKeyCode"], up["windowsVirtualKeyCode"] = int(r-32), int(r-32)
	case r >= 'A' && r <= 'Z':
		down["code"], up["code"] = "Key"+s, "Key"+s
		down["windowsVirtualKeyCode"], up["windowsVirtualKeyCode"] = int(r), int(r)
		down["modifiers"], up["modifiers"] = 8, 8 // shift
	case r >= '0' && r <= '9':
		down["code"], up["code"] = "Digit"+s, "Digit"+s
		down["windowsVirtualKeyCode"], up["windowsVirtualKeyCode"] = int(r), int(r)
	case r == ' ':
		down["code"], up["code"] = "Space", "Space"
		down["windowsVirtualKeyCode"], up["windowsVirtualKeyCode"] = 32, 32
	}
	if err := x.c.call(ctx, x.sid, "Input.dispatchKeyEvent", down, nil); err != nil {
		return err
	}
	return x.c.call(ctx, x.sid, "Input.dispatchKeyEvent", up, nil)
}

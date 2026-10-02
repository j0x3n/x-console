package reminders

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// Fixed artwork comes from the existing notification assets. User-defined
// common glyphs are drawn below, without loading fonts or remote images.
//
//go:embed icon_assets/*.png
var iconAssets embed.FS

var emojiGlyphs = map[string]string{
	"💧": "water", "💦": "water", "🥤": "water", "🍶": "water", "🧃": "water",
	"👁": "eyes", "👀": "eyes", "👓": "eyes",
	"🚶": "move", "🏃": "move", "👟": "move", "🦶": "move",
	"💊": "medicine", "🩺": "medicine", "💉": "medicine",
	"📖": "book", "📚": "book", "📝": "pen", "✍": "pen", "✏": "pen",
	"🌙": "moon", "😴": "moon", "🛏": "moon", "💪": "dumbbell", "🏋": "dumbbell",
	"🍎": "apple", "🍏": "apple", "🥗": "salad", "🚴": "bike", "🚲": "bike",
	"🏊": "swim", "🧘": "meditate", "🦷": "tooth", "🎸": "guitar",
	"☀": "weather", "⛅": "weather", "☁": "weather", "❤": "habit", "💖": "habit", "💗": "habit",
	"🔔": "reminder", "⏰": "reminder", "📅": "calendar", "📆": "calendar", "📧": "mail", "✉": "mail",
}

func glyph(icon string) string {
	icon = strings.ReplaceAll(strings.TrimSpace(icon), "\ufe0f", "")
	if g, ok := emojiGlyphs[icon]; ok {
		return g
	}
	if customGlyph(icon) {
		return icon
	}
	if _, err := iconAssets.ReadFile("icon_assets/" + icon + ".png"); err == nil {
		return icon
	}
	return ""
}

func (m *Module) iconSignature(name string) string {
	mac := hmac.New(sha256.New, m.d.Config.MasterKey)
	mac.Write([]byte("x-console.notify.icon.v1:" + name))
	return hex.EncodeToString(mac.Sum(nil))
}
func (m *Module) iconURL(name string) string {
	return "/api/v1/notify/icons/" + name + ".png?sig=" + m.iconSignature(name+".png")
}

func (m *Module) GetNotifyIcon(w http.ResponseWriter, r *http.Request, name string, p api.GetNotifyIconParams) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	want, _ := hex.DecodeString(m.iconSignature(name))
	got, err := hex.DecodeString(p.Sig)
	if err != nil || !hmac.Equal(want, got) {
		httpx.Fail(w, r, httpx.NewError(403, "invalid_signature", "图标签名不对"))
		return
	}
	if len(name) > 160 || !strings.HasSuffix(name, ".png") {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	m.iconMu.Lock()
	defer m.iconMu.Unlock()
	raw, ok := m.icons[name]
	if !ok {
		raw, err = m.renderIcon(strings.TrimSuffix(name, ".png"))
		if err != nil {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		if len(m.iconOrder) >= 256 {
			delete(m.icons, m.iconOrder[0])
			m.iconOrder = m.iconOrder[1:]
		}
		m.icons[name] = raw
		m.iconOrder = append(m.iconOrder, name)
		m.iconDraws++
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}

var progressName = regexp.MustCompile(`^habit-[1-9][0-9]*-progress-([0-9]+(?:\.[0-9]+)?)-([0-9]+(?:\.[0-9]+)?)-([a-z]+)$`)

func (m *Module) renderIcon(name string) ([]byte, error) {
	g := strings.TrimPrefix(name, "icon-")
	progress := -1.0
	if match := progressName.FindStringSubmatch(name); match != nil {
		done, _ := strconv.ParseFloat(match[1], 64)
		target, _ := strconv.ParseFloat(match[2], 64)
		if target <= 0 || math.IsInf(done, 0) || math.IsInf(target, 0) {
			return nil, fmt.Errorf("invalid progress")
		}
		progress = math.Min(1, done/target)
		g = match[3]
	} else if strings.HasPrefix(name, "emoji-") {
		var text strings.Builder
		for _, part := range strings.Split(strings.TrimPrefix(name, "emoji-"), "-") {
			n, err := strconv.ParseInt(part, 16, 32)
			if err != nil {
				return nil, err
			}
			text.WriteRune(rune(n))
		}
		g = glyph(text.String())
	}
	if g == "" || glyph(g) == "" {
		return nil, fmt.Errorf("unsupported icon")
	}
	img, err := glyphImage(g)
	if err != nil {
		return nil, err
	}
	if progress >= 0 {
		base := image.NewRGBA(image.Rect(0, 0, 192, 192))
		bg := img.RGBAAt(96, 28)
		for y := 0; y < 192; y++ {
			for x := 0; x < 192; x++ {
				c := bg
				if math.Hypot(float64(x)-96, float64(y)-96) > 94 {
					c = color.RGBA{}
				}
				base.SetRGBA(x, y, c)
			}
		}
		// Scale the central glyph to leave room for an antialiased ring.
		for y := 0; y < 112; y++ {
			for x := 0; x < 112; x++ {
				base.SetRGBA(x+40, y+40, img.RGBAAt(x*192/112, y*192/112))
			}
		}
		for y := 0; y < 192; y++ {
			for x := 0; x < 192; x++ {
				dx, dy := float64(x)+.5-96, float64(y)+.5-96
				r := math.Hypot(dx, dy)
				alpha := math.Max(0, math.Min(1, 5.5-math.Abs(r-81)))
				if alpha == 0 {
					continue
				}
				angle := math.Atan2(dy, dx) + math.Pi/2
				if angle < 0 {
					angle += 2 * math.Pi
				}
				level := 0.25
				if angle <= progress*2*math.Pi && progress > 0 {
					level = 1
				}
				old := base.RGBAAt(x, y)
				a := alpha * level
				base.SetRGBA(x, y, color.RGBA{uint8(float64(old.R)*(1-a) + 255*a), uint8(float64(old.G)*(1-a) + 255*a), uint8(float64(old.B)*(1-a) + 255*a), 255})
			}
		}
		img = base
	}
	var out bytes.Buffer
	err = png.Encode(&out, img)
	return out.Bytes(), err
}

func glyphImage(g string) (*image.RGBA, error) {
	if customGlyph(g) {
		return drawGlyph(g), nil
	}
	raw, err := iconAssets.ReadFile("icon_assets/" + g + ".png")
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	out := image.NewRGBA(image.Rect(0, 0, 192, 192))
	for y := 0; y < 192; y++ {
		for x := 0; x < 192; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out, nil
}

func number(v any) float64 {
	switch x := v.(type) {
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case float64:
		return x
	case json.Number:
		n, _ := x.Float64()
		return n
	}
	return 0
}
func text(v any) string { x, _ := v.(string); return x }

func (m *Module) pushIcons(ctx context.Context, n notify.Stored) (string, string) {
	kind := "app"
	for _, entry := range []struct{ prefix, icon string }{
		{"host.", "server"}, {"monitor.", "monitor"}, {"site.", "monitor"}, {"domain.", "monitor"}, {"tls.", "monitor"}, {"cert.", "monitor"}, {"subscription.", "monitor"},
		{"mail.", "mail"}, {"github.", "repo"}, {"repo.", "repo"}, {"forgejo.", "repo"}, {"ai_agent.", "agent"}, {"coding_task.", "agent"},
		{"weather.", "weather"}, {"reminder.", "reminder"}, {"habit.", "habit"}, {"workout.", "habit"}, {"brief.", "brief"}, {"calendar.", "calendar"}, {"focus.", "calendar"},
		{"drive.", "drive"}, {"backup.", "drive"}, {"router.", "router"}, {"ha.", "home"}, {"homeassistant.", "home"},
	} {
		if strings.HasPrefix(n.Kind, entry.prefix) {
			kind = entry.icon
			break
		}
	}
	icon := "/icons/notify/" + kind + ".png"
	badge := "/icons/notify/" + kind + "-badge.png"
	item := text(n.Data["icon"])
	template := text(n.Data["template"])
	if strings.HasPrefix(n.Kind, "reminder.") {
		if id := int64(number(n.Data["reminderId"])); id > 0 {
			if r, err := m.q.GetReminder(ctx, id); err == nil {
				item = r.Icon
			}
		}
	}
	g := glyph(item)
	if g == "" {
		g = glyph(template)
	}
	if strings.HasPrefix(n.Kind, "habit.") && g == "" {
		g = "habit"
	}
	if g != "" {
		if strings.HasPrefix(n.Kind, "habit.") && number(n.Data["target"]) > 0 {
			done := math.Max(0, number(n.Data["done"]))
			target := number(n.Data["target"])
			if !math.IsInf(done, 0) && !math.IsNaN(done) && !math.IsInf(target, 0) && !math.IsNaN(target) {
				name := fmt.Sprintf("habit-%d-progress-%s-%s-%s", int64(number(n.Data["habitId"])), strconv.FormatFloat(math.Min(done, target), 'f', -1, 64), strconv.FormatFloat(target, 'f', -1, 64), g)
				if len(name) <= 150 {
					icon = m.iconURL(name)
				}
			}
		} else if item != "" {
			if _, ok := emojiGlyphs[strings.ReplaceAll(item, "\ufe0f", "")]; ok {
				parts := []string{}
				for _, r := range item {
					parts = append(parts, fmt.Sprintf("%x", r))
				}
				icon = m.iconURL("emoji-" + strings.Join(parts, "-"))
			} else {
				icon = m.iconURL("icon-" + g)
			}
		} else {
			icon = "/icons/notify/" + g + ".png"
		}
	}
	return icon, badge
}

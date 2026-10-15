package brief

// B163: with send mode "wake" the brief goes out when the me module reports a
// wake-up (event me.woke, guessed ones included). Time is then the latest
// time: the minute tick still sends at Time when no wake-up came. A day gets
// one brief either way.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

// earliestWake is the first local hour a wake-up sends the brief. Waking up
// in the small hours and going back to sleep must not use up the day's brief.
const earliestWake = 4

func (m *Module) followWake(ctx context.Context) {
	ch, cancel := m.d.Bus.Subscribe("me.woke", 16)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				raw, _ := json.Marshal(ev.Data)
				var woke contracts.MeWoke
				if json.Unmarshal(raw, &woke) != nil {
					continue
				}
				if err := m.onWake(ctx, time.Now()); err != nil {
					m.d.Log.Warn("brief after wake-up", "err", err)
				}
			}
		}
	}()
}

// onWake sends today's brief for a wake-up at now when the send mode asks
// for it.
func (m *Module) onWake(ctx context.Context, now time.Time) error {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()
	cfg, err := m.load(ctx)
	if err != nil {
		return err
	}
	if !wakeSends(cfg, now, m.d.Config.Location, m.sentOn(ctx, now)) {
		return nil
	}
	_, err = m.generateAndSend(ctx, cfg, now)
	return err
}

// wakeSends decides whether a wake-up at now sends the brief. Past the latest
// time's send window the day is over: the tick has sent it or it was missed.
func wakeSends(cfg config, now time.Time, loc *time.Location, sentToday bool) bool {
	if !cfg.Enabled || cfg.SendMode != sendOnWake || sentToday || now.In(loc).Hour() < earliestWake {
		return false
	}
	latest, ok := plannedAt(cfg.Time, now, loc)
	return ok && now.Before(latest.Add(sendWindow))
}

// ---- weather lines (B163) ----

// sameTemp is how many degrees apart still counts as "about the same".
const sameTemp = 3

// compareHigh compares today's forecast high with yesterday's measured one.
func compareHigh(today, yesterday float64) string {
	diff := int(math.Round(today - yesterday))
	switch {
	case diff > sameTemp:
		return fmt.Sprintf("最高 %s°，比昨天高 %d°。", num(today), diff)
	case diff < -sameTemp:
		return fmt.Sprintf("最高 %s°，比昨天低 %d°。", num(today), -diff)
	}
	return fmt.Sprintf("最高 %s°，和昨天差不多。", num(today))
}

// rainSpans turns the hours of today that have rain into "15 点到 18 点有雨".
// hours must be sorted. It returns "" without rain.
func rainSpans(hours []int) string {
	var spans []string
	for i := 0; i < len(hours); {
		j := i
		for j+1 < len(hours) && hours[j+1] == hours[j]+1 {
			j++
		}
		if end := hours[j] + 1; end == 24 {
			spans = append(spans, fmt.Sprintf("%d 点以后", hours[i]))
		} else {
			spans = append(spans, fmt.Sprintf("%d 点到 %d 点", hours[i], end))
		}
		i = j + 1
	}
	if len(spans) == 0 {
		return ""
	}
	return strings.Join(spans, "、") + "有雨。"
}

// rainHours lists the hours left today whose hourly forecast has rain.
func (m *Module) rainHours(ctx context.Context, c qwConfig, place api.BriefLocation, now time.Time, loc *time.Location) ([]int, error) {
	var raw struct {
		Hourly []struct {
			Time   string  `json:"fxTime"`
			Precip flexNum `json:"precip"`
		} `json:"hourly"`
	}
	if err := m.qwGet(ctx, c, "/v7/weather/24h?location="+deref(place.Id)+"&lang=zh", &raw); err != nil {
		return nil, err
	}
	l := now.In(loc)
	from := time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), 0, 0, 0, loc)
	end := time.Date(l.Year(), l.Month(), l.Day()+1, 0, 0, 0, 0, loc)
	var hours []int
	for _, h := range raw.Hourly {
		t, ok := qwTime(h.Time)
		if !ok || t.Before(from) || !t.Before(end) || !h.Precip.Set || h.Precip.V <= 0 {
			continue
		}
		if hour := t.In(loc).Hour(); len(hours) == 0 || hours[len(hours)-1] < hour {
			hours = append(hours, hour)
		}
	}
	return hours, nil
}

// weatherMore is what the brief adds below the weather line: the high against
// yesterday, the hours with rain and the warnings in force. Each part that
// cannot be read is left out.
func (m *Module) weatherMore(ctx context.Context, place api.BriefLocation, w api.Weather, now time.Time) []string {
	c, err := m.requireQWeather(ctx)
	if err != nil {
		return nil
	}
	if place, err = m.resolveLocation(ctx, c, place); err != nil {
		return nil
	}
	var lines []string
	if y, err := m.fetchYesterday(ctx, c, place); err == nil {
		lines = append(lines, compareHigh(w.High, y.High))
	} else {
		m.d.Log.Info("brief: no weather of yesterday", "err", err)
	}
	if hours, err := m.rainHours(ctx, c, place, now, m.d.Config.Location); err == nil {
		if s := rainSpans(hours); s != "" {
			lines = append(lines, s)
		}
	} else {
		m.d.Log.Info("brief: no hourly forecast", "err", err)
	}
	if items, err := m.fetchWarnings(ctx, c, place); err == nil {
		for _, it := range activeWarnings(items) {
			lines = append(lines, "预警："+it.Title+"。")
		}
	}
	return lines
}

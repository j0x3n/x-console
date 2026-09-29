package hosts

import (
	"context"
	"encoding/json"
)

// followIntervals listens for host.metrics_interval, which the browser
// connection handler publishes when the interval of a viewed host changes.
// At 1 second or faster every sample goes to the browsers, otherwise the
// usual throttle applies.
func (m *Module) followIntervals(ctx context.Context) {
	ch, cancel := m.d.Bus.Subscribe("host.metrics_interval", 64)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-ch:
				var data struct {
					HostID string `json:"hostId"`
					Ms     int    `json:"ms"`
				}
				raw, err := json.Marshal(ev.Data)
				if err != nil || json.Unmarshal(raw, &data) != nil || data.HostID == "" {
					continue
				}
				m.metrics.setFast(data.HostID, data.Ms > 0 && data.Ms <= 1000)
			}
		}
	}()
}

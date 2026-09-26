package calendar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"

	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/db"
)

// syncInterval is how often every enabled calendar is fetched again.
const syncInterval = 15 * time.Minute

// maxICSSize bounds a downloaded calendar.
const maxICSSize = 20 << 20

// syncAll refreshes every enabled calendar. Errors are stored per calendar.
func (m *Module) syncAll(ctx context.Context) error {
	cals, err := m.q.ListCalendars(ctx)
	if err != nil {
		return err
	}
	for _, c := range cals {
		if c.Enabled == 1 {
			if _, err := m.syncOne(ctx, c.ID); err != nil && ctx.Err() != nil {
				return err
			}
		}
	}
	return nil
}

// syncOne downloads one calendar and replaces its events. A failed download
// keeps the old events and records the error on the calendar.
func (m *Module) syncOne(ctx context.Context, id int64) (db.Calendar, error) {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	cal, err := m.q.GetCalendar(ctx, id)
	if err != nil {
		return cal, notFound(err)
	}
	events, fetchErr := m.fetch(ctx, cal)
	if fetchErr == nil {
		fetchErr = m.replaceEvents(ctx, cal.ID, events)
	}
	msg := ""
	if fetchErr != nil {
		msg = fetchErr.Error()
		m.d.Log.Warn("calendar sync failed", "calendar", cal.ID, "err", msg)
	}
	now := time.Now().UTC()
	updated, err := m.q.SetCalendarSync(ctx, db.SetCalendarSyncParams{LastSyncedAt: &now, LastError: msg, ID: cal.ID})
	if err != nil {
		return cal, notFound(err)
	}
	m.d.Bus.Publish("calendar.synced", map[string]any{"id": cal.ID, "error": msg})
	return updated, nil
}

func (m *Module) replaceEvents(ctx context.Context, calendarID int64, events []parsedEvent) error {
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	if err := q.DeleteCalendarEvents(ctx, calendarID); err != nil {
		return err
	}
	for _, e := range events {
		allDay := int64(0)
		if e.AllDay {
			allDay = 1
		}
		if err := q.InsertCalendarEvent(ctx, db.InsertCalendarEventParams{
			CalendarID: calendarID, Uid: e.UID, Title: e.Title, StartsAt: e.Start, EndsAt: e.End, AllDay: allDay,
			Tzid: e.TZID, Location: e.Location, Description: e.Description, Rrule: e.RRule,
			Rdates: encodeTimes(e.RDates), Exdates: encodeTimes(e.ExDates), RecurrenceID: e.RecurrenceID,
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// fetch downloads and parses the events of a calendar.
func (m *Module) fetch(ctx context.Context, cal db.Calendar) ([]parsedEvent, error) {
	password, err := m.password(cal)
	if err != nil {
		return nil, errors.New("密码无法解密，请重新填写")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if cal.Kind == kindCalDAV {
		return m.fetchCalDAV(ctx, cal.Url, cal.Username, password)
	}
	data, err := m.fetchICS(ctx, cal.Url, cal.Username, password)
	if err != nil {
		return nil, err
	}
	return parseICS(data)
}

func (m *Module) password(cal db.Calendar) (string, error) {
	if cal.Secret == "" {
		return "", nil
	}
	return m.d.Secrets.Open(cal.Secret)
}

// cleanErr drops the URL from transport errors: subscription links often
// carry a private token.
func cleanErr(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return fmt.Errorf("连接失败: %w", uerr.Err)
	}
	return err
}

func (m *Module) fetchICS(ctx context.Context, rawURL, username, password string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, icsURL(rawURL), nil)
	if err != nil {
		return nil, errors.New("地址格式不对")
	}
	req.Header.Set("User-Agent", "X-Console")
	req.Header.Set("Accept", "text/calendar, */*")
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxICSSize+1))
	if err != nil {
		return nil, cleanErr(err)
	}
	if len(data) > maxICSSize {
		return nil, errors.New("日历文件太大了")
	}
	if !bytes.Contains(data[:min(len(data), 4096)], []byte("BEGIN:VCALENDAR")) {
		return nil, errors.New("下载到的内容不是日历文件")
	}
	return data, nil
}

// icsURL turns webcal:// links into https://.
func icsURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(raw, "webcals://"); ok {
		return "https://" + rest
	}
	if rest, ok := strings.CutPrefix(raw, "webcal://"); ok {
		return "https://" + rest
	}
	return raw
}

// fetchCalDAV reads the events of a CalDAV calendar. The URL may point at a
// calendar collection, a calendar home, or just the server; in the last case
// the calendars are discovered through the current user principal.
func (m *Module) fetchCalDAV(ctx context.Context, rawURL, username, password string) ([]parsedEvent, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return nil, errors.New("地址格式不对")
	}
	var hc webdav.HTTPClient = m.http
	if username != "" || password != "" {
		hc = webdav.HTTPClientWithBasicAuth(m.http, username, password)
	}
	client, err := caldav.NewClient(hc, u.String())
	if err != nil {
		return nil, errors.New("地址格式不对")
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	cals, err := client.FindCalendars(ctx, path)
	if err != nil || len(cals) == 0 {
		principal, perr := client.FindCurrentUserPrincipal(ctx)
		if perr != nil {
			if err == nil {
				err = perr
			}
			return nil, fmt.Errorf("找不到日历: %w", cleanErr(err))
		}
		home, herr := client.FindCalendarHomeSet(ctx, principal)
		if herr != nil {
			return nil, fmt.Errorf("找不到日历: %w", cleanErr(herr))
		}
		if cals, err = client.FindCalendars(ctx, home); err != nil {
			return nil, fmt.Errorf("找不到日历: %w", cleanErr(err))
		}
	}
	var out []parsedEvent
	found := false
	for _, c := range cals {
		if !supportsEvents(c) {
			continue
		}
		found = true
		objs, err := client.QueryCalendar(ctx, c.Path, &caldav.CalendarQuery{
			CompRequest: caldav.CalendarCompRequest{Name: "VCALENDAR", AllProps: true, AllComps: true},
			CompFilter:  caldav.CompFilter{Name: "VCALENDAR", Comps: []caldav.CompFilter{{Name: "VEVENT"}}},
		})
		if err != nil {
			return nil, fmt.Errorf("读取日历失败: %w", cleanErr(err))
		}
		for _, o := range objs {
			if o.Data == nil {
				continue
			}
			var buf bytes.Buffer
			if err := ical.NewEncoder(&buf).Encode(o.Data); err != nil {
				continue
			}
			events, err := parseICS(buf.Bytes())
			if err != nil {
				continue
			}
			out = append(out, events...)
		}
	}
	if !found {
		return nil, errors.New("这个地址下没有日程日历")
	}
	return out, nil
}

func supportsEvents(c caldav.Calendar) bool {
	if len(c.SupportedComponentSet) == 0 {
		return true
	}
	for _, name := range c.SupportedComponentSet {
		if strings.EqualFold(name, "VEVENT") {
			return true
		}
	}
	return false
}

package calendar

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/google/uuid"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar/db"
)

type eventValues struct {
	calendarID                              int64
	uid, title, tzid, location, description string
	start, end                              time.Time // Wall clock in tzid, or UTC date for all-day events.
	allDay                                  bool
	href, etag                              string
}

func (m *Module) validEvent(in api.EventInput) (eventValues, error) {
	v := eventValues{calendarID: in.CalendarId, uid: uuid.NewString(), title: strings.TrimSpace(in.Title),
		location: strings.TrimSpace(deref(in.Location)), description: deref(in.Description), allDay: in.AllDay}
	if v.title == "" || len([]rune(v.title)) > 200 {
		return v, httpx.Invalid("日程标题需为 1 到 200 个字")
	}
	if v.allDay {
		if in.StartDate == nil || in.EndDate == nil {
			return v, httpx.Invalid("全天日程需要开始和结束日期")
		}
		var err error
		v.start, err = time.Parse(time.DateOnly, *in.StartDate)
		if err != nil {
			return v, httpx.Invalid("开始日期格式不对")
		}
		v.end, err = time.Parse(time.DateOnly, *in.EndDate)
		if err != nil {
			return v, httpx.Invalid("结束日期格式不对")
		}
	} else {
		if in.Start == nil || in.End == nil {
			return v, httpx.Invalid("日程需要开始和结束时间")
		}
		loc := m.d.Config.Location
		v.start, v.end = wallOf(in.Start.In(loc)), wallOf(in.End.In(loc))
		v.tzid = loc.String()
	}
	if !v.end.After(v.start) {
		return v, httpx.Invalid("结束时间要晚于开始时间")
	}
	return v, nil
}

func (m *Module) writableCalendar(ctx context.Context, id int64) (db.Calendar, error) {
	c, err := m.q.GetCalendar(ctx, id)
	if err != nil {
		return c, notFound(err)
	}
	if c.Kind != kindLocal && c.Kind != kindCalDAV {
		return c, httpx.Invalid("这个日历只能查看，不能修改日程")
	}
	return c, nil
}

func (m *Module) editableEvent(ctx context.Context, id int64) (db.CalendarEvent, db.Calendar, error) {
	e, err := m.q.GetCalendarEvent(ctx, id)
	if err != nil {
		return e, db.Calendar{}, notFound(err)
	}
	c, err := m.writableCalendar(ctx, e.CalendarID)
	if err != nil {
		return e, c, err
	}
	if e.Rrule != "" || e.Rdates != "[]" || e.RecurrenceID != nil {
		return e, c, httpx.Invalid("重复日程暂时不能修改")
	}
	return e, c, nil
}

func (m *Module) eventAPI(e db.CalendarEvent, c db.Calendar) api.CalendarEvent {
	loc := zoneOr(e.Tzid, m.d.Config.Location)
	if e.AllDay == 1 {
		loc = m.d.Config.Location
	}
	start, end := inZone(e.StartsAt, loc), inZone(e.EndsAt, loc)
	writable := c.Kind == kindLocal || c.Kind == kindCalDAV
	out := api.CalendarEvent{Id: itoa(e.ID) + "-" + itoa(start.Unix()), EventId: e.ID,
		CalendarId: c.ID, Calendar: c.Name, Color: c.Color, Title: e.Title,
		Start: start, End: end, AllDay: e.AllDay == 1, Location: e.Location,
		Description: e.Description, Writable: &writable}
	if out.AllDay {
		s, endDate := e.StartsAt.Format(time.DateOnly), e.EndsAt.Format(time.DateOnly)
		out.StartDate, out.EndDate = &s, &endDate
	}
	return out
}

func (m *Module) insertEvent(ctx context.Context, v eventValues) (db.CalendarEvent, error) {
	return m.q.InsertCalendarEvent(ctx, db.InsertCalendarEventParams{
		CalendarID: v.calendarID, Uid: v.uid, Title: v.title, StartsAt: v.start,
		EndsAt: v.end, AllDay: boolInt(v.allDay), Tzid: v.tzid,
		Location: v.location, Description: v.description, Rrule: "", Rdates: "[]", Exdates: "[]",
		Href: v.href, Etag: v.etag,
	})
}

func (m *Module) CreateCalendarEvent(w http.ResponseWriter, r *http.Request) {
	var in api.EventInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := m.validEvent(in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := m.writableCalendar(r.Context(), v.calendarID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if c.Kind == kindCalDAV {
		m.syncMu.Lock()
		defer m.syncMu.Unlock()
		v.href, v.etag, err = m.putRemote(r.Context(), c, v, "", "")
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	e, err := m.insertEvent(r.Context(), v)
	m.d.Audit.Record(r.Context(), "calendar.event.create", v.uid, map[string]any{"calendarId": c.ID}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := m.eventAPI(e, c)
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		// Saved already: log instead of failing, so the client does not retry.
		if err := files.Claim(r.Context(), "calendar", e.ID, e.Description); err != nil {
			m.d.Log.Error("claim images failed", "id", e.ID, "err", err)
		}
	}
	m.d.Bus.Publish("calendar.event_changed", out)
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) patchEvent(e db.CalendarEvent, p api.EventPatch) (eventValues, error) {
	allDay := e.AllDay == 1
	if p.AllDay != nil {
		allDay = *p.AllDay
	}
	loc := zoneOr(e.Tzid, m.d.Config.Location)
	if e.AllDay == 1 {
		loc = m.d.Config.Location
	}
	start, end := inZone(e.StartsAt, loc), inZone(e.EndsAt, loc)
	in := api.EventInput{CalendarId: e.CalendarID, Title: e.Title, AllDay: allDay,
		Location: &e.Location, Description: &e.Description}
	if p.CalendarId != nil {
		in.CalendarId = *p.CalendarId
	}
	if p.Title != nil {
		in.Title = *p.Title
	}
	if p.Location != nil {
		in.Location = p.Location
	}
	if p.Description != nil {
		in.Description = p.Description
	}
	if allDay {
		if e.AllDay == 1 {
			s, d := e.StartsAt.Format(time.DateOnly), e.EndsAt.Format(time.DateOnly)
			in.StartDate, in.EndDate = &s, &d
		}
		if p.StartDate != nil {
			in.StartDate = p.StartDate
		}
		if p.EndDate != nil {
			in.EndDate = p.EndDate
		}
	} else {
		if e.AllDay == 0 {
			in.Start, in.End = &start, &end
		}
		if p.Start != nil {
			in.Start = p.Start
		}
		if p.End != nil {
			in.End = p.End
		}
	}
	v, err := m.validEvent(in)
	v.uid, v.href, v.etag = e.Uid, e.Href, e.Etag
	return v, err
}

func (m *Module) UpdateCalendarEvent(w http.ResponseWriter, r *http.Request, id int64) {
	var p api.EventPatch
	if err := httpx.Decode(r, &p); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, old, err := m.editableEvent(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := m.patchEvent(e, p)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next, err := m.writableCalendar(r.Context(), v.calendarID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if old.Kind == kindCalDAV || next.Kind == kindCalDAV {
		m.syncMu.Lock()
		defer m.syncMu.Unlock()
		e, old, err = m.editableEvent(r.Context(), id)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		v, err = m.patchEvent(e, p)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		next, err = m.writableCalendar(r.Context(), v.calendarID)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if old.Kind == kindCalDAV && (e.Href == "" || e.Etag == "") {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "calendar_conflict", "日程缺少远端版本，请先同步"))
		return
	}
	if next.ID == old.ID {
		if next.Kind == kindCalDAV {
			v.href, v.etag, err = m.putRemote(r.Context(), next, v, e.Href, e.Etag)
		}
	} else {
		if next.Kind == kindCalDAV {
			v.href, v.etag, err = m.putRemote(r.Context(), next, v, "", "")
		} else {
			v.href, v.etag = "", ""
		}
		if err == nil && old.Kind == kindCalDAV {
			err = m.deleteRemote(r.Context(), old, e.Href, e.Etag)
			if err != nil && next.Kind == kindCalDAV {
				_ = m.deleteRemote(r.Context(), next, v.href, v.etag)
			}
		}
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := m.q.UpdateCalendarEvent(r.Context(), db.UpdateCalendarEventParams{
		CalendarID: v.calendarID, Title: v.title, StartsAt: v.start, EndsAt: v.end,
		AllDay: boolInt(v.allDay), Tzid: v.tzid, Location: v.location,
		Description: v.description, Href: v.href, Etag: v.etag, ID: id,
	})
	m.d.Audit.Record(r.Context(), "calendar.event.update", itoa(id), map[string]any{"calendarId": next.ID}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := m.eventAPI(updated, next)
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		// Saved already: log instead of failing, so the client does not retry.
		if err := files.Claim(r.Context(), "calendar", id, updated.Description); err != nil {
			m.d.Log.Error("claim images failed", "id", id, "err", err)
		}
	}
	m.d.Bus.Publish("calendar.event_changed", out)
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) DeleteCalendarEvent(w http.ResponseWriter, r *http.Request, id int64) {
	e, c, err := m.editableEvent(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if c.Kind == kindCalDAV {
		m.syncMu.Lock()
		defer m.syncMu.Unlock()
		e, c, err = m.editableEvent(r.Context(), id)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if err = m.deleteRemote(r.Context(), c, e.Href, e.Etag); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	_, err = m.q.DeleteCalendarEvent(r.Context(), id)
	m.d.Audit.Record(r.Context(), "calendar.event.delete", itoa(id), map[string]any{"calendarId": c.ID}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		if err := files.DeleteOwned(r.Context(), "calendar", id); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	m.d.Bus.Publish("calendar.event_changed", map[string]any{"eventId": id, "calendarId": c.ID})
	httpx.NoContent(w)
}

// A conditional client adds the headers missing from go-webdav's PUT API.
type conditionalClient struct {
	webdav.HTTPClient
	header, value string
}

func (c conditionalClient) Do(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPut {
		r.Header.Set(c.header, c.value)
	}
	return c.HTTPClient.Do(r)
}

func (m *Module) remoteClient(c db.Calendar, header, value string) (*caldav.Client, error) {
	password, err := m.password(c)
	if err != nil {
		return nil, httpx.Invalid("密码无法解密，请重新填写")
	}
	var hc webdav.HTTPClient = m.http
	if c.Username != "" || password != "" {
		hc = webdav.HTTPClientWithBasicAuth(m.http, c.Username, password)
	}
	if header != "" {
		hc = conditionalClient{hc, header, value}
	}
	return caldav.NewClient(hc, c.Url)
}

func remoteError(err error) error {
	if err == nil {
		return nil
	}
	if strings.HasPrefix(err.Error(), "412 ") || err.Error() == "HTTP 412" {
		return httpx.NewError(http.StatusConflict, "calendar_conflict", "日程在别处改过了，刷新后再改")
	}
	return httpx.NewError(http.StatusBadGateway, "caldav_failed", "CalDAV 写入失败: "+cleanErr(err).Error())
}

func eventICal(v eventValues) *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//X Console//Calendar//EN")
	e := ical.NewEvent()
	e.Props.SetText(ical.PropUID, v.uid)
	e.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	e.Props.SetText(ical.PropSummary, v.title)
	e.Props.SetText(ical.PropLocation, v.location)
	e.Props.SetText(ical.PropDescription, v.description)
	if v.allDay {
		e.Props.SetDate(ical.PropDateTimeStart, v.start)
		e.Props.SetDate(ical.PropDateTimeEnd, v.end)
	} else {
		loc := zoneOr(v.tzid, time.UTC)
		e.Props.SetDateTime(ical.PropDateTimeStart, inZone(v.start, loc).UTC())
		e.Props.SetDateTime(ical.PropDateTimeEnd, inZone(v.end, loc).UTC())
	}
	cal.Children = append(cal.Children, e.Component)
	return cal
}

func (m *Module) collection(ctx context.Context, c db.Calendar) (string, error) {
	client, err := m.remoteClient(c, "", "")
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(c.Url)
	start := u.Path
	if start == "" {
		start = "/"
	}
	cals, err := client.FindCalendars(ctx, start)
	if err != nil || len(cals) == 0 {
		principal, e := client.FindCurrentUserPrincipal(ctx)
		if e != nil {
			return "", remoteError(e)
		}
		home, e := client.FindCalendarHomeSet(ctx, principal)
		if e != nil {
			return "", remoteError(e)
		}
		cals, err = client.FindCalendars(ctx, home)
		if err != nil {
			return "", remoteError(err)
		}
	}
	for _, item := range cals {
		if supportsEvents(item) {
			return item.Path, nil
		}
	}
	return "", httpx.Invalid("这个地址下没有日程日历")
}

func (m *Module) putRemote(ctx context.Context, c db.Calendar, v eventValues, href, etag string) (string, string, error) {
	newObject := href == ""
	if href == "" {
		collection, err := m.collection(ctx, c)
		if err != nil {
			return "", "", err
		}
		href = path.Join(collection, v.uid+".ics")
		if !strings.HasPrefix(href, "/") {
			href = "/" + href
		}
	}
	if etag == "" && !newObject {
		return "", "", httpx.NewError(http.StatusConflict, "calendar_conflict", "日程缺少远端版本，请先同步")
	}
	header, value := "If-None-Match", "*"
	if etag != "" {
		header, value = "If-Match", etag
	}
	client, err := m.remoteClient(c, header, value)
	if err != nil {
		return "", "", err
	}
	obj, err := client.PutCalendarObject(ctx, href, eventICal(v))
	if err != nil {
		return "", "", remoteError(err)
	}
	if obj.ETag == "" {
		obj, err = client.GetCalendarObject(ctx, href)
		if err != nil {
			return "", "", remoteError(err)
		}
	}
	return href, obj.ETag, nil
}

func (m *Module) deleteRemote(ctx context.Context, c db.Calendar, href, etag string) error {
	if href == "" || etag == "" {
		return httpx.NewError(http.StatusConflict, "calendar_conflict", "日程缺少远端版本，请先同步")
	}
	u, err := url.Parse(c.Url)
	if err != nil {
		return httpx.Invalid("CalDAV 地址格式不对")
	}
	u.Path, u.RawPath, u.RawQuery = href, "", ""
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("If-Match", etag)
	password, err := m.password(c)
	if err != nil {
		return httpx.Invalid("密码无法解密，请重新填写")
	}
	if c.Username != "" || password != "" {
		req.SetBasicAuth(c.Username, password)
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return remoteError(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusPreconditionFailed {
		return remoteError(fmt.Errorf("HTTP 412"))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return remoteError(fmt.Errorf("HTTP %d", resp.StatusCode))
	}
	return nil
}

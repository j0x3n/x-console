package calendar

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/emersion/go-webdav"
)

const calendarQueryReport = `<?xml version="1.0" encoding="UTF-8"?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop><D:getetag/><C:calendar-data/></D:prop>
  <C:filter><C:comp-filter name="VCALENDAR"><C:comp-filter name="VEVENT"/></C:comp-filter></C:filter>
</C:calendar-query>`

type calendarReport struct {
	XMLName   xml.Name                 `xml:"DAV: multistatus"`
	Responses []calendarReportResponse `xml:"DAV: response"`
}

type calendarReportResponse struct {
	Hrefs     []string `xml:"DAV: href"`
	Status    string   `xml:"DAV: status"`
	Propstats []struct {
		Status string `xml:"DAV: status"`
		Prop   struct {
			Data string `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
			ETag string `xml:"DAV: getetag"`
		} `xml:"DAV: prop"`
	} `xml:"DAV: propstat"`
}

type calendarReportResult struct {
	events               []parsedEvent
	hasEntries, readable bool
}

func davStatusOK(status string) bool {
	fields := strings.Fields(status)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/") {
		return false
	}
	code, err := strconv.Atoi(fields[1])
	return err == nil && code >= 200 && code < 300
}

// queryCalendar 逐条读取 REPORT，失败的资源不会阻止其他资源同步。
func queryCalendar(ctx context.Context, hc webdav.HTTPClient, endpoint *url.URL, collection string) (calendarReportResult, error) {
	var result calendarReportResult
	u := *endpoint
	u.Path, u.RawPath, u.RawQuery, u.Fragment = collection, "", "", ""
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = path.Join(endpoint.Path, collection)
	}
	req, err := http.NewRequestWithContext(ctx, "REPORT", u.String(), strings.NewReader(calendarQueryReport))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")
	resp, err := hc.Do(req)
	if err != nil {
		return result, cleanErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return result, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxICSSize+1))
	if err != nil {
		return result, cleanErr(err)
	}
	if len(raw) > maxICSSize {
		return result, errors.New("日历文件太大了")
	}
	var report calendarReport
	if err := xml.Unmarshal(raw, &report); err != nil {
		return result, errors.New("日历响应格式不对")
	}
	result.hasEntries = len(report.Responses) > 0
	for _, item := range report.Responses {
		if len(item.Hrefs) != 1 || item.Status != "" && !davStatusOK(item.Status) {
			continue
		}
		href, err := url.Parse(strings.TrimSpace(item.Hrefs[0]))
		if err != nil || href.Path == "" {
			continue
		}
		data, etag := "", ""
		for _, props := range item.Propstats {
			if !davStatusOK(props.Status) {
				continue
			}
			if strings.TrimSpace(props.Prop.Data) != "" {
				data = props.Prop.Data
			}
			if props.Prop.ETag != "" {
				etag = strings.TrimSpace(props.Prop.ETag)
			}
		}
		if data == "" {
			continue
		}
		events, err := parseICS([]byte(data))
		if err != nil {
			continue
		}
		result.readable = true
		if !strings.HasPrefix(href.Path, "/") {
			href.Path = path.Join(u.Path, href.Path)
		}
		for i := range events {
			events[i].Href, events[i].ETag = href.Path, etag
		}
		result.events = append(result.events, events...)
	}
	return result, nil
}

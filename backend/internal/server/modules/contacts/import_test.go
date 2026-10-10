package contacts_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts/api"
)

const appleCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:u-zhang\r\nFN:张三\r\nN:张;三;;;\r\n" +
	"BDAY;X-APPLE-OMIT-YEAR=1604:1604-05-12\r\n" +
	"TEL;type=CELL:+86 138 0000 0000\r\nEMAIL;type=HOME:zhang@example.com\r\n" +
	"item1.X-ABDATE;type=pref:2015-06-01\r\nitem1.X-ABLABEL:_$!<Anniversary>!$_\r\n" +
	"item2.X-ABDATE:2000-02-29\r\nitem2.X-ABLABEL:领证\r\nNOTE:高中同学\r\nEND:VCARD\r\n"

const plainCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:u-li\r\nFN:Li Si\r\nTEL:010-1234\r\nEND:VCARD\r\n"

func TestParseVCards(t *testing.T) {
	raw := appleCard + plainCard +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nN:Wang;Wu;;;\r\nBDAY:19900812\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nN:李;四;;;\r\nBDAY:--0301\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nTEL:123\r\nEND:VCARD\r\n"
	names, dates, bad := contacts.ParseVCards(raw)
	want := []string{"张三", "Li Si", "Wu Wang", "李四"}
	if strings.Join(names, "|") != strings.Join(want, "|") || bad != 1 {
		t.Fatalf("names %v bad %d", names, bad)
	}
	got := strings.Join(dates[0], ",")
	// 苹果补的假年份 1604 不要，没有年份的写成 月-日
	if got != "im-bday=birthday:生日:05-12,im-d0=anniversary:纪念日:2015-06-01,im-d1=other:领证:2000-02-29" {
		t.Fatalf("apple dates: %s", got)
	}
	if strings.Join(dates[2], ",") != "im-bday=birthday:生日:1990-08-12" || strings.Join(dates[3], ",") != "im-bday=birthday:生日:03-01" {
		t.Fatalf("other dates: %v", dates)
	}
}

func (r *rig) importFile(t *testing.T, query, content string) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "contacts.vcf")
	_, _ = fw.Write([]byte(content))
	_ = mw.Close()
	req, err := http.NewRequest(http.MethodPost, r.env.URL("/contacts/import"+query), &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := r.env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func (r *rig) importOK(t *testing.T, query, content string) api.ContactImportResult {
	t.Helper()
	status, raw := r.importFile(t, query, content)
	if status != http.StatusOK {
		t.Fatalf("import: %d %s", status, raw)
	}
	var out api.ContactImportResult
	if err := jsonUnmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestImportVCardCreatesAndUpdates(t *testing.T) {
	r := setup(t)
	res := r.importOK(t, "", appleCard+plainCard+"BEGIN:VCARD\r\nVERSION:3.0\r\nTEL:1\r\nEND:VCARD\r\n")
	if res.Total != 3 || res.Created != 2 || res.Updated != 0 || res.Skipped != 1 {
		t.Fatalf("first import: %+v", res)
	}
	out := r.list(t, "")
	var zhang api.Contact
	for _, c := range out.Items {
		if c.Name == "张三" {
			zhang = c
		}
	}
	if zhang.Source != "vcard" || len(zhang.Phones) != 1 || zhang.Emails[0] != "zhang@example.com" || zhang.Notes != "高中同学" || len(zhang.Events) != 3 {
		t.Fatalf("imported contact: %+v", zhang)
	}
	if zhang.Events[0].Date != "05-12" || zhang.Events[0].Years != nil {
		t.Fatalf("birthday without year: %+v", zhang.Events[0])
	}

	// 同一个文件再导入一次：没有变化
	if res := r.importOK(t, "", appleCard+plainCard); res.Created != 0 || res.Updated != 0 {
		t.Fatalf("second import: %+v", res)
	}

	// 自己改过的设置不被覆盖，文件里变了的电话会更新
	g := api.ContactGroupFamily
	r.env.MustDo(http.MethodPatch, fmt.Sprintf("/contacts/%d", zhang.Id), api.ContactPatch{Group: &g, Notes: strPtr("我改的备注")}, nil)
	changed := strings.Replace(appleCard, "+86 138 0000 0000", "+86 139 1111 1111", 1)
	if res := r.importOK(t, "", changed); res.Updated != 1 || res.Created != 0 {
		t.Fatalf("third import: %+v", res)
	}
	var got api.Contact
	r.env.MustDo(http.MethodGet, fmt.Sprintf("/contacts/%d", zhang.Id), nil, &got)
	if got.Group != g || got.Notes != "我改的备注" || got.Phones[0] != "+86 139 1111 1111" {
		t.Fatalf("after update: %+v", got)
	}
}

func TestImportMergesByNameAndFiltersDates(t *testing.T) {
	r := setup(t)
	manual := r.create(t, map[string]any{"name": "Li Si", "phones": []string{"mine-1"}})
	if res := r.importOK(t, "?onlyWithDates=true", plainCard); res.Skipped != 1 || res.Created != 0 || res.Updated != 0 {
		t.Fatalf("only with dates: %+v", res)
	}
	if res := r.importOK(t, "", plainCard); res.Created != 0 || res.Updated != 1 {
		t.Fatalf("merge by name: %+v", res)
	}
	var got api.Contact
	r.env.MustDo(http.MethodGet, fmt.Sprintf("/contacts/%d", manual.Id), nil, &got)
	// 手动建的联系人保留自己的号码，再加上导入的
	if len(got.Phones) != 2 || got.Phones[0] != "mine-1" || got.Phones[1] != "010-1234" || got.Source != "vcard" {
		t.Fatalf("merged: %+v", got)
	}
	if n := len(r.list(t, "").Items); n != 1 {
		t.Fatalf("no duplicate: %d", n)
	}
}

func TestImportRejectsBadFiles(t *testing.T) {
	r := setup(t)
	if status, _ := r.importFile(t, "", "这不是通讯录"); status != http.StatusBadRequest {
		t.Fatalf("not a vcard: %d", status)
	}
	req, _ := http.NewRequest(http.MethodPost, r.env.URL("/contacts/import"), strings.NewReader("x"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := r.env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("no file: %d", resp.StatusCode)
	}
}

// fakeICloud answers the CardDAV requests the sync makes. The home set is an
// absolute address, the way iCloud answers.
func fakeICloud(t *testing.T, homeHost string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	// homeHost 不为空时，目录地址指向一台解析不了的主机，像 iCloud 的分区主机在这台服务器上解析不了
	host := func(self, other string) string {
		if other != "" {
			return "https://" + other
		}
		return self
	}
	ms := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav">`+body+`</d:multistatus>`)
	}
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "me@icloud.com" || p != "abcd-efgh-ijkl-mnop" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == "PROPFIND" && r.URL.Path == "/":
			ms(w, `<d:response><d:href>/</d:href><d:propstat><d:prop><d:current-user-principal><d:href>/1234/principal/</d:href></d:current-user-principal></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
		case r.Method == "PROPFIND" && r.URL.Path == "/1234/principal/":
			ms(w, `<d:response><d:href>/1234/principal/</d:href><d:propstat><d:prop><c:addressbook-home-set><d:href>`+host(srv.URL, homeHost)+`/1234/carddavhome/</d:href></c:addressbook-home-set></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
		case r.Method == "PROPFIND" && r.URL.Path == "/1234/carddavhome/":
			ms(w, `<d:response><d:href>/1234/carddavhome/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`+
				`<d:response><d:href>/1234/carddavhome/card/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/><c:addressbook/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
		case r.Method == "REPORT" && r.URL.Path == "/1234/carddavhome/card/":
			card := func(name, vcf string) string {
				return `<d:response><d:href>/1234/carddavhome/card/` + name + `.vcf</d:href><d:propstat><d:prop><c:address-data>` +
					strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(vcf) + `</c:address-data></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`
			}
			ms(w, card("a", appleCard)+card("b", plainCard))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestICloudSync(t *testing.T) {
	r := setup(t)
	fake := fakeICloud(t, "")
	contacts.SetHTTPClient(r.m, fake.Client())
	put := func(pass string) (int, []byte) {
		return r.env.Do(http.MethodPut, "/contacts/sync", map[string]any{"username": "me@icloud.com", "password": pass, "server": fake.URL}, nil)
	}

	if status, _ := put("abcd-efgh-ijkl-mnop"); status != http.StatusForbidden {
		t.Fatalf("set without elevation: %d", status)
	}
	r.env.Elevate()
	if status, raw := put("wrong"); status != http.StatusBadRequest || !strings.Contains(string(raw), "应用专用密码") {
		t.Fatalf("wrong password: %d %s", status, raw)
	}
	var st api.ContactSyncStatus
	r.env.MustDo(http.MethodGet, "/contacts/sync", nil, &st)
	if st.Configured {
		t.Fatalf("a failed login must not be saved: %+v", st)
	}

	status, raw := put("abcd-efgh-ijkl-mnop")
	if status != http.StatusOK {
		t.Fatalf("set: %d %s", status, raw)
	}
	if strings.Contains(string(raw), "abcd-efgh") {
		t.Fatalf("the password came back: %s", raw)
	}
	r.env.MustDo(http.MethodGet, "/contacts/sync", nil, &st)
	if !st.Configured || st.Created != 2 || st.Total != 2 || st.LastSyncAt == nil || st.LastError != nil {
		t.Fatalf("status: %+v", st)
	}
	out := r.list(t, "")
	if len(out.Items) != 2 {
		t.Fatalf("synced contacts: %d", len(out.Items))
	}
	for _, c := range out.Items {
		if c.Source != "icloud" {
			t.Fatalf("source: %+v", c)
		}
	}
	// 密码只加密存，库里看不到明文
	var stored string
	r.env.App.Deps.DB.QueryRow("SELECT group_concat(value) FROM settings WHERE key LIKE 'contacts.sync%'").Scan(&stored)
	if strings.Contains(stored, "abcd-efgh") {
		t.Fatalf("password stored in plain text: %s", stored)
	}

	// 再同步一次：没有变化
	r.env.MustDo(http.MethodPost, "/contacts/sync/run", nil, &st)
	if st.Created != 0 || st.Updated != 0 || st.Total != 2 {
		t.Fatalf("second sync: %+v", st)
	}
	// 定时任务不到间隔不重复同步
	if err := contacts.SyncJob(r.m, t.Context()); err != nil {
		t.Fatal(err)
	}

	if status, _ := r.env.Do(http.MethodDelete, "/contacts/sync", nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	r.env.MustDo(http.MethodGet, "/contacts/sync", nil, &st)
	if st.Configured {
		t.Fatalf("after delete: %+v", st)
	}
	if status, _ := r.env.Do(http.MethodPost, "/contacts/sync/run", nil, nil); status != http.StatusConflict {
		t.Fatalf("run without account: %d", status)
	}
	if n := len(r.list(t, "").Items); n != 2 {
		t.Fatalf("contacts stay after the account is removed: %d", n)
	}
}

func TestICloudSyncRefusesPlainHTTP(t *testing.T) {
	r := setup(t)
	r.env.Elevate()
	status, _ := r.env.Do(http.MethodPut, "/contacts/sync", map[string]any{"username": "a", "password": "b", "server": "http://example.com"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("http server: %d", status)
	}
}

func jsonUnmarshal(raw []byte, v any) error { return json.Unmarshal(raw, v) }

func strPtr(s string) *string { return &s }

// iCloud 返回的分区主机（pNN-contacts.icloud.com）在这台服务器上解析不了时，
// 改用最初的地址访问同一个路径。
func TestICloudSyncFallsBackWhenPartitionHostDoesNotResolve(t *testing.T) {
	r := setup(t)
	fake := fakeICloud(t, "p231-contacts.no-such-host.invalid")
	contacts.SetHTTPClient(r.m, fake.Client())
	r.env.Elevate()
	status, raw := r.env.Do(http.MethodPut, "/contacts/sync", map[string]any{"username": "me@icloud.com", "password": "abcd-efgh-ijkl-mnop", "server": fake.URL}, nil)
	if status != http.StatusOK {
		t.Fatalf("set: %d %s", status, raw)
	}
	var st api.ContactSyncStatus
	r.env.MustDo(http.MethodGet, "/contacts/sync", nil, &st)
	if !st.Configured || st.Created != 2 {
		t.Fatalf("status: %+v", st)
	}
}

package backup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	storageapi "github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const maxRuns = 10

var clockText = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// s3Fields is the custom S3 setup, kept in the settings without its secret.
type s3Fields struct {
	Endpoint    string `json:"endpoint"`
	Region      string `json:"region"`
	Bucket      string `json:"bucket"`
	Prefix      string `json:"prefix"`
	AccessKeyID string `json:"accessKeyId"`
	PathStyle   bool   `json:"pathStyle"`
}

// runRecord is one line of the history of automatic backups.
type runRecord struct {
	At        time.Time `json:"at"`
	Ok        bool      `json:"ok"`
	BackupID  string    `json:"backupId,omitempty"`
	SizeBytes int64     `json:"sizeBytes,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// settingsData is stored under keySettings.
type settingsData struct {
	Enabled   bool         `json:"enabled"`
	Frequency string       `json:"frequency"`
	Time      string       `json:"time"`
	Weekday   int          `json:"weekday"`
	Keep      int          `json:"keep"`
	Target    string       `json:"target"`
	S3        s3Fields     `json:"s3"`
	WebDAV    webdavFields `json:"webdav"`
	GDrive    gdriveFields `json:"gdrive"`
	// LastAttemptAt is when the last automatic run started, successful or not.
	// A run is due when the latest scheduled time is after it.
	LastAttemptAt time.Time   `json:"lastAttemptAt"`
	LastRuns      []runRecord `json:"lastRuns"`
}

func defaultSettings() settingsData {
	return settingsData{Frequency: "daily", Time: "03:00", Keep: 14, Target: targetStorage,
		WebDAV: webdavFields{Folder: defaultWebDAVFolder}, GDrive: gdriveFields{FolderName: defaultGDriveFolder}}
}

func (m *Module) loadSettings(ctx context.Context) (settingsData, error) {
	s := defaultSettings()
	if err := m.d.Settings.Get(ctx, keySettings, &s); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return s, err
	}
	return s, nil
}

var errNoS3 = httpx.NewError(http.StatusPreconditionFailed, "integration_not_configured", "还没有可用的 S3，请先在存储设置或这里填写")

// remote opens the location of the automatic backups.
func (m *Module) remote(ctx context.Context) (target, error) {
	s, err := m.loadSettings(ctx)
	if err != nil {
		return target{}, err
	}
	return m.open(ctx, s, newSecrets{})
}

// slot is the latest time a run was due at or before now.
func (s settingsData) slot(now time.Time, loc *time.Location) time.Time {
	var h, min int
	fmt.Sscanf(s.Time, "%d:%d", &h, &min)
	at := now.In(loc)
	day := at.Day()
	if s.Frequency == "weekly" {
		day -= (int(at.Weekday()) - s.Weekday + 7) % 7
	}
	slot := time.Date(at.Year(), at.Month(), day, h, min, 0, 0, loc)
	if slot.After(now) {
		back := 1
		if s.Frequency == "weekly" {
			back = 7
		}
		slot = time.Date(slot.Year(), slot.Month(), slot.Day()-back, h, min, 0, 0, loc)
	}
	return slot
}

// nextSlot is the first scheduled time after now.
func (s settingsData) nextSlot(now time.Time, loc *time.Location) time.Time {
	last := s.slot(now, loc)
	step := 1
	if s.Frequency == "weekly" {
		step = 7
	}
	return time.Date(last.Year(), last.Month(), last.Day()+step, last.Hour(), last.Minute(), 0, 0, loc)
}

// tick runs every minute and starts a backup when one is due.
func (m *Module) tick(ctx context.Context) error {
	s, err := m.loadSettings(ctx)
	if err != nil || !s.Enabled {
		return err
	}
	now := m.now()
	if !s.LastAttemptAt.Before(s.slot(now, m.d.Config.Location)) {
		return nil
	}
	if err := m.runAuto(audit.WithActor(ctx, "system:backup")); err != nil && !errors.Is(err, errBusy) {
		return err
	}
	return nil
}

// runAuto starts a backup to S3 in the background.
func (m *Module) runAuto(ctx context.Context) error {
	s, err := m.loadSettings(ctx)
	if err != nil {
		return err
	}
	dest, err := m.open(ctx, s, newSecrets{})
	if err != nil {
		return err
	}
	j, err := m.begin(api.BackupJobKindAuto)
	if err != nil {
		return err
	}
	bg := context.WithoutCancel(ctx)
	if err := m.update(bg, func(s *settingsData) { s.LastAttemptAt = m.now() }); err != nil {
		m.end(j, err)
		return err
	}
	go func() {
		name, size, err := m.create(bg, kindAuto, dest.store, remoteFolder, true, j)
		if err == nil {
			j.set(func(v *api.BackupJob) { v.BackupId = &name })
			m.pruneRemote(bg, dest, s.Keep)
			m.rememberFolder(bg, s, dest.gdrive)
		}
		m.recordRun(bg, name, size, err)
		m.d.Audit.Record(bg, "backup.auto", name, map[string]any{"bytes": size}, err)
		if err != nil {
			m.notifyFailure(bg, err)
		} else {
			m.d.Bus.Publish("backup.created", map[string]any{"id": name})
		}
		m.end(j, err)
	}()
	return nil
}

// update changes the saved settings under a lock.
func (m *Module) update(ctx context.Context, f func(*settingsData)) error {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	s, err := m.loadSettings(ctx)
	if err != nil {
		return err
	}
	f(&s)
	return m.d.Settings.Set(ctx, keySettings, s)
}

func (m *Module) recordRun(ctx context.Context, name string, size int64, runErr error) {
	rec := runRecord{At: m.now().UTC(), Ok: runErr == nil, BackupID: name, SizeBytes: size}
	if runErr != nil {
		rec.Error = runErr.Error()
	}
	err := m.update(ctx, func(s *settingsData) {
		s.LastRuns = append([]runRecord{rec}, s.LastRuns...)
		if len(s.LastRuns) > maxRuns {
			s.LastRuns = s.LastRuns[:maxRuns]
		}
	})
	if err != nil {
		m.log().Warn("backup: record run", "error", err)
	}
}

func (m *Module) notifyFailure(ctx context.Context, cause error) {
	title := "自动备份失败"
	if errors.Is(cause, files.ErrGDriveAuth) {
		title = files.ErrGDriveAuth.Error()
	}
	_, err := m.d.Notify.Send(ctx, notify.Notification{Kind: "backup.failed", Title: title, Body: cause.Error(),
		Link: "/settings/backup", Priority: notify.PriorityNormal, Source: "backup"})
	if err != nil {
		m.log().Warn("backup: send failure notice", "error", err)
	}
}

// pruneRemote deletes the oldest automatic backups beyond keep.
func (m *Module) pruneRemote(ctx context.Context, dest target, keep int) {
	all, err := scan(ctx, dest.store, remoteFolder, dest.loc)
	if err != nil {
		m.log().Warn("backup: prune remote", "location", dest.loc, "error", err)
		return
	}
	var autos []entry
	for _, e := range all {
		if e.kind() == kindAuto {
			autos = append(autos, e)
		}
	}
	sort.Slice(autos, func(i, j int) bool { return autos[i].createdAt().After(autos[j].createdAt()) })
	for i := keep; i < len(autos); i++ {
		if err := autos[i].remove(ctx); err != nil {
			m.log().Warn("backup: remove old remote backup", "name", autos[i].key, "error", err)
		}
	}
}

// RunBackupNow is POST /backups/run.
func (m *Module) RunBackupNow(w http.ResponseWriter, r *http.Request) {
	if err := m.runAuto(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.mu.Lock()
	j := m.cur
	m.mu.Unlock()
	httpx.JSON(w, http.StatusAccepted, j.snapshot())
}

// lastRun is the type the generated API uses for one history line.
type lastRun = struct {
	At        time.Time `json:"at"`
	BackupId  *string   `json:"backupId,omitempty"`
	Error     *string   `json:"error,omitempty"`
	Ok        bool      `json:"ok"`
	SizeBytes *int64    `json:"sizeBytes,omitempty"`
}

// secretsSet says which secrets are saved.
type secretsSet struct {
	s3, webdav, gdrive, token bool
}

func (m *Module) secretsSet(ctx context.Context) secretsSet {
	return secretsSet{s3: m.secret(ctx, keyS3Secret) != "", webdav: m.secret(ctx, keyWebDAVPassword) != "",
		gdrive: m.secret(ctx, keyGDriveSecret) != "", token: m.secret(ctx, keyGDriveToken) != ""}
}

// view turns saved settings into the API answer.
func (m *Module) view(s settingsData, set secretsSet, now time.Time) api.BackupSettings {
	out := api.BackupSettings{Enabled: s.Enabled, Frequency: api.BackupSettingsFrequency(s.Frequency), Time: s.Time,
		Weekday: s.Weekday, Keep: s.Keep, Target: api.BackupSettingsTarget(s.Target)}
	if s.S3 != (s3Fields{}) || set.s3 {
		out.S3 = &storageapi.StorageS3{Endpoint: s.S3.Endpoint, Region: s.S3.Region, Bucket: s.S3.Bucket, Prefix: s.S3.Prefix,
			AccessKeyId: s.S3.AccessKeyID, HasSecret: set.s3, PathStyle: s.S3.PathStyle}
	}
	out.Webdav = &api.BackupWebdav{Url: s.WebDAV.URL, Username: s.WebDAV.Username, Folder: s.WebDAV.Folder, PasswordSet: set.webdav}
	out.Gdrive = &api.BackupGdrive{ClientId: s.GDrive.ClientID, FolderName: s.GDrive.FolderName, SecretSet: set.gdrive, Authorized: set.token}
	if set.token && s.GDrive.Account != "" {
		out.Gdrive.Account = ptr(s.GDrive.Account)
	}
	if s.Enabled {
		next := s.nextSlot(now, m.d.Config.Location).UTC()
		out.NextRunAt = &next
	}
	out.LastRuns = make([]lastRun, 0, len(s.LastRuns))
	for _, run := range s.LastRuns {
		item := lastRun{At: run.At, Ok: run.Ok}
		if run.BackupID != "" {
			item.BackupId = ptr(run.BackupID)
		}
		if run.SizeBytes != 0 {
			item.SizeBytes = ptr(run.SizeBytes)
		}
		if run.Error != "" {
			item.Error = ptr(run.Error)
		}
		out.LastRuns = append(out.LastRuns, item)
	}
	return out
}

// GetBackupSettings is GET /backups/settings.
func (m *Module) GetBackupSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := m.loadSettings(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.view(s, m.secretsSet(ctx), m.now()))
}

// PutBackupSettings is PUT /backups/settings.
func (m *Module) PutBackupSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.BackupSettingsInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.saveSettings(ctx, in)
	m.d.Audit.Record(ctx, "backup.settings", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) saveSettings(ctx context.Context, in api.BackupSettingsInput) (api.BackupSettings, error) {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	s, err := m.loadSettings(ctx)
	if err != nil {
		return api.BackupSettings{}, err
	}
	old := s
	p := apply(&s, in)
	if err := m.validate(ctx, s, p); err != nil {
		return api.BackupSettings{}, err
	}
	if s.Enabled && !old.Enabled {
		// Switching it on does not start a backup for a time that has already passed.
		s.LastAttemptAt = m.now()
	}
	if s.GDrive.ClientID != old.GDrive.ClientID {
		// The token belongs to the old client.
		if err := m.d.Settings.Delete(ctx, keyGDriveToken); err != nil {
			return api.BackupSettings{}, err
		}
		s.GDrive.Account, s.GDrive.FolderID = "", ""
	}
	if s.GDrive.FolderName != old.GDrive.FolderName {
		s.GDrive.FolderID = ""
	}
	for key, v := range map[string]string{keyS3Secret: p.s3, keyWebDAVPassword: p.webdav, keyGDriveSecret: p.gdrive} {
		if v == "" {
			continue
		}
		if err := m.d.Settings.SetSecret(ctx, key, v); err != nil {
			return api.BackupSettings{}, err
		}
	}
	if err := m.d.Settings.Set(ctx, keySettings, s); err != nil {
		return api.BackupSettings{}, err
	}
	return m.view(s, m.secretsSet(ctx), m.now()), nil
}

// apply copies the fields of in onto s and returns the new secrets in it.
func apply(s *settingsData, in api.BackupSettingsInput) newSecrets {
	var p newSecrets
	if in.Enabled != nil {
		s.Enabled = *in.Enabled
	}
	if in.Frequency != nil {
		s.Frequency = string(*in.Frequency)
	}
	if in.Time != nil {
		s.Time = *in.Time
	}
	if in.Weekday != nil {
		s.Weekday = *in.Weekday
	}
	if in.Keep != nil {
		s.Keep = *in.Keep
	}
	if in.Target != nil {
		s.Target = string(*in.Target)
	}
	if in.S3 != nil {
		applyS3(&s.S3, *in.S3)
		if in.S3.SecretAccessKey != nil {
			p.s3 = *in.S3.SecretAccessKey
		}
	}
	if w := in.Webdav; w != nil {
		set(&s.WebDAV.URL, w.Url)
		set(&s.WebDAV.Username, w.Username)
		set(&s.WebDAV.Folder, w.Folder)
		if w.Password != nil {
			p.webdav = *w.Password
		}
	}
	if g := in.Gdrive; g != nil {
		set(&s.GDrive.ClientID, g.ClientId)
		set(&s.GDrive.FolderName, g.FolderName)
		if g.ClientSecret != nil {
			p.gdrive = strings.TrimSpace(*g.ClientSecret)
		}
	}
	s.WebDAV.URL = strings.TrimSpace(s.WebDAV.URL)
	s.WebDAV.Folder = strings.Trim(strings.TrimSpace(s.WebDAV.Folder), "/")
	s.GDrive.ClientID = strings.TrimSpace(s.GDrive.ClientID)
	s.GDrive.FolderName = strings.TrimSpace(s.GDrive.FolderName)
	return p
}

func set(dst *string, v *string) {
	if v != nil {
		*dst = *v
	}
}

func applyS3(dst *s3Fields, in storageapi.StorageS3Input) {
	if in.Endpoint != nil {
		dst.Endpoint = *in.Endpoint
	}
	if in.Region != nil {
		dst.Region = *in.Region
	}
	if in.Bucket != nil {
		dst.Bucket = *in.Bucket
	}
	if in.Prefix != nil {
		dst.Prefix = *in.Prefix
	}
	if in.AccessKeyId != nil {
		dst.AccessKeyID = *in.AccessKeyId
	}
	if in.PathStyle != nil {
		dst.PathStyle = *in.PathStyle
	}
}

// validate rejects settings that cannot work. The location is only tried
// when the automatic backup is on.
func (m *Module) validate(ctx context.Context, s settingsData, p newSecrets) error {
	if s.Frequency != "daily" && s.Frequency != "weekly" {
		return httpx.Invalid("频率只能是每天或每周")
	}
	if !clockText.MatchString(s.Time) {
		return httpx.Invalid("时间格式应为 HH:MM，比如 03:00")
	}
	if s.Weekday < 0 || s.Weekday > 6 {
		return httpx.Invalid("星期要在 0 到 6 之间，0 是周日")
	}
	if s.Keep < 1 || s.Keep > 365 {
		return httpx.Invalid("保留份数要在 1 到 365 之间")
	}
	switch s.Target {
	case targetStorage, targetCustom, targetWebDAV, targetGDrive:
	default:
		return httpx.Invalid("备份位置不正确")
	}
	if s.WebDAV.URL != "" {
		if err := files.ValidateWebDAV(files.WebDAVConfig{URL: s.WebDAV.URL, Folder: s.WebDAV.Folder}); err != nil {
			return httpx.Invalid(err.Error())
		}
	}
	if s.GDrive.FolderName == "" {
		return httpx.Invalid("Google Drive 的文件夹名不能为空")
	}
	if !s.Enabled {
		return nil
	}
	if err := m.tryTarget(ctx, s, p); err != nil {
		return httpx.Invalid(err.Error())
	}
	return nil
}

// tryTarget opens the location and writes, reads and deletes a small file.
// The error is a short message for the settings page.
func (m *Module) tryTarget(ctx context.Context, s settingsData, p newSecrets) error {
	t, err := m.open(ctx, s, p)
	if errors.Is(err, errNoS3) {
		if s.Target == targetStorage {
			return errors.New("存储设置里还没有 S3")
		}
		return errors.New("自动备份的 S3 设置没有填完整")
	}
	var herr *httpx.Error
	if errors.As(err, &herr) {
		return errors.New(herr.Message)
	}
	if err != nil {
		return err
	}
	if err := t.check(ctx); err != nil {
		return errors.New("连接测试没通过：" + err.Error())
	}
	return nil
}

// TestBackupTarget is POST /backups/target/test.
func (m *Module) TestBackupTarget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.BackupSettingsInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := m.loadSettings(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := apply(&s, in)
	out := api.BackupTargetTest{Ok: true, Message: "连接正常，可以读写"}
	if err := m.tryTarget(ctx, s, p); err != nil {
		out = api.BackupTargetTest{Ok: false, Message: err.Error()}
	}
	httpx.JSON(w, http.StatusOK, out)
}

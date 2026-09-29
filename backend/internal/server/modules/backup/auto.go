package backup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	storageapi "github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	targetStorage = "storage" // the S3 of the storage settings
	targetCustom  = "custom"  // an S3 of its own
	maxRuns       = 10
)

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
	Enabled   bool     `json:"enabled"`
	Frequency string   `json:"frequency"`
	Time      string   `json:"time"`
	Weekday   int      `json:"weekday"`
	Keep      int      `json:"keep"`
	Target    string   `json:"target"`
	S3        s3Fields `json:"s3"`
	// LastAttemptAt is when the last automatic run started, successful or not.
	// A run is due when the latest scheduled time is after it.
	LastAttemptAt time.Time   `json:"lastAttemptAt"`
	LastRuns      []runRecord `json:"lastRuns"`
}

func defaultSettings() settingsData {
	return settingsData{Frequency: "daily", Time: "03:00", Keep: 14, Target: targetStorage}
}

func (m *Module) loadSettings(ctx context.Context) (settingsData, error) {
	s := defaultSettings()
	if err := m.d.Settings.Get(ctx, keySettings, &s); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return s, err
	}
	return s, nil
}

func (m *Module) customSecret(ctx context.Context) string {
	var secret string
	if err := m.d.Settings.Get(ctx, keyS3Secret, &secret); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.log().Warn("backup: S3 secret cannot be read, treating it as not set", "error", err)
	}
	return secret
}

var errNoS3 = httpx.NewError(http.StatusPreconditionFailed, "integration_not_configured", "还没有可用的 S3，请先在存储设置或这里填写")

// s3Config is the bucket the automatic backups go to.
// secret is a new secret key that is not saved yet, or "" to use the saved one.
func (m *Module) s3Config(ctx context.Context, s settingsData, secret string) (files.S3Config, bool, error) {
	if s.Target == targetCustom {
		if secret == "" {
			secret = m.customSecret(ctx)
		}
		c := files.S3Config{Endpoint: s.S3.Endpoint, Region: s.S3.Region, Bucket: s.S3.Bucket, Prefix: s.S3.Prefix,
			AccessKeyID: s.S3.AccessKeyID, SecretAccessKey: secret, PathStyle: s.S3.PathStyle}
		return c, c.Endpoint != "" && c.Bucket != "" && c.AccessKeyID != "" && secret != "", nil
	}
	return storage.S3Config(ctx, m.d.Settings, m.log())
}

// remote opens the bucket for automatic backups.
func (m *Module) remote(ctx context.Context) (files.Store, error) {
	s, err := m.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	return m.remoteFor(ctx, s)
}

func (m *Module) remoteFor(ctx context.Context, s settingsData) (files.Store, error) {
	cfg, ok, err := m.s3Config(ctx, s, "")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errNoS3
	}
	return files.NewS3(cfg)
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
	dest, err := m.remoteFor(ctx, s)
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
		name, size, err := m.create(bg, kindAuto, dest, remoteFolder, true, j)
		if err == nil {
			j.set(func(v *api.BackupJob) { v.BackupId = &name })
			m.pruneRemote(bg, dest, s.Keep)
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
	_, err := m.d.Notify.Send(ctx, notify.Notification{Kind: "backup.failed", Title: "自动备份失败", Body: cause.Error(),
		Link: "/settings/backup", Priority: notify.PriorityNormal, Source: "backup"})
	if err != nil {
		m.log().Warn("backup: send failure notice", "error", err)
	}
}

// pruneRemote deletes the oldest automatic backups beyond keep.
func (m *Module) pruneRemote(ctx context.Context, dest files.Store, keep int) {
	all, err := scan(ctx, dest, remoteFolder, api.S3)
	if err != nil {
		m.log().Warn("backup: prune S3", "error", err)
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
			m.log().Warn("backup: remove old S3 backup", "name", autos[i].key, "error", err)
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

// view turns saved settings into the API answer.
func (m *Module) view(s settingsData, secret string, now time.Time) api.BackupSettings {
	out := api.BackupSettings{Enabled: s.Enabled, Frequency: api.BackupSettingsFrequency(s.Frequency), Time: s.Time,
		Weekday: s.Weekday, Keep: s.Keep, Target: api.BackupSettingsTarget(s.Target)}
	if s.S3 != (s3Fields{}) || secret != "" {
		out.S3 = &storageapi.StorageS3{Endpoint: s.S3.Endpoint, Region: s.S3.Region, Bucket: s.S3.Bucket, Prefix: s.S3.Prefix,
			AccessKeyId: s.S3.AccessKeyID, HasSecret: secret != "", PathStyle: s.S3.PathStyle}
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
	httpx.JSON(w, http.StatusOK, m.view(s, m.customSecret(ctx), m.now()))
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
	wasEnabled := s.Enabled
	secret := m.customSecret(ctx)
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
	newSecret := ""
	if in.S3 != nil {
		applyS3(&s.S3, *in.S3)
		if in.S3.SecretAccessKey != nil && *in.S3.SecretAccessKey != "" {
			newSecret = *in.S3.SecretAccessKey
			secret = newSecret
		}
	}
	if err := m.validate(ctx, s, secret); err != nil {
		return api.BackupSettings{}, err
	}
	if s.Enabled && !wasEnabled {
		// Switching it on does not start a backup for a time that has already passed.
		s.LastAttemptAt = m.now()
	}
	if newSecret != "" {
		if err := m.d.Settings.SetSecret(ctx, keyS3Secret, newSecret); err != nil {
			return api.BackupSettings{}, err
		}
	}
	if err := m.d.Settings.Set(ctx, keySettings, s); err != nil {
		return api.BackupSettings{}, err
	}
	return m.view(s, secret, m.now()), nil
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

// validate rejects settings that cannot work. A bucket is only tried when the
// automatic backup is on.
func (m *Module) validate(ctx context.Context, s settingsData, customSecret string) error {
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
	if s.Target != targetStorage && s.Target != targetCustom {
		return httpx.Invalid("备份位置不正确")
	}
	if !s.Enabled {
		return nil
	}
	cfg, ok, err := m.s3Config(ctx, s, customSecret)
	if err != nil {
		return err
	}
	if !ok {
		if s.Target == targetStorage {
			return httpx.Invalid("存储设置里还没有 S3")
		}
		return httpx.Invalid("自动备份的 S3 设置没有填完整")
	}
	remote, err := files.NewS3(cfg)
	if err != nil {
		return httpx.Invalid(err.Error())
	}
	if err := remote.Check(ctx); err != nil {
		return httpx.Invalid("连接测试没通过：" + err.Error())
	}
	return nil
}

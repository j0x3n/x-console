// Package backup exports the whole site (database and files) into one
// package, keeps automatic backups in S3 and restores a package (B25). See
// docs/specs/B24-B25.md.
//
// A restore cannot swap the database under a running server, so it is done in
// two steps: the package is unpacked into data/restore-tmp/, then the server
// stops. The supervisor (Docker, systemd) starts it again, and ApplyPending
// puts the new database in place before it is opened. The files are copied
// into the file store by the module once the server is up.
package backup

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/maintenance"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// ServiceKey finds the module in module.Registry, for tests.
const ServiceKey = "backup.service"

// Settings keys.
const (
	keySettings = "backup.settings"  // settingsData, plain JSON
	keyS3Secret = "backup.s3_secret" // encrypted secret key of the custom S3
	keyJob      = "backup.job"       // the last finished job, api.BackupJob
)

// Kinds of backup, as written into sidecar files.
const (
	kindManual     = "manual"
	kindAuto       = "auto"
	kindPreRestore = "pre-restore"
	kindUploaded   = "uploaded"
)

const (
	keepLocal    = 5        // manual and pre-restore backups kept on this machine, each
	maxUpload    = 20 << 30 // largest package accepted by /backups/upload
	remoteFolder = "backups/"
)

// backupID is what a package file name may look like.
var backupID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.tar\.gz$`)

var errBusy = httpx.NewError(http.StatusConflict, "conflict", "已经有备份或恢复在进行，等它完成再试")

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	local files.Local // data/backups

	now  func() time.Time
	exit func() // stops the process so a staged restore gets applied

	mu   sync.Mutex // guards cur and busy
	cur  *job
	busy bool

	settingsMu sync.Mutex // serialises read-modify-write of the settings
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module. When a restore is waiting for its file phase, that
// starts here.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, local: files.Local{Root: d.Config.BackupsDir()}, now: time.Now, exit: stopProcess}
	if pending(d.Config.RestoreDir()) {
		m.startFinishRestore()
	}
	d.Scheduler.Every("backup.auto", time.Minute, m.tick)
	module.Provide[*Module](d.Registry, ServiceKey, m)
	module.Provide[contracts.RemoteUser](d.Registry, contracts.RemoteUserKey, m)
	module.Provide[contracts.StorageReporter](d.Registry, contracts.MaintenanceStoragePrefix+"backup", maintenance.StoreReporter{Store: m.local, Registry: d.Registry, Key: "backups", Label: "本地备份", Location: "local"})
	return m, nil
}

// Start moves the B63 drive accounts to the storage module once (B69). It
// runs after every module was built, so the storage accounts are there.
func (m *Module) Start(ctx context.Context) error {
	if err := m.migrateRemotes(ctx); err != nil {
		// 迁移失败不影响启动，下次启动再试
		m.log().Error("backup: move the drive accounts to storage", "error", err)
	}
	return nil
}

// stopProcess asks the server to shut down as it does on Ctrl-C.
func stopProcess() {
	if p, err := os.FindProcess(os.Getpid()); err == nil {
		_ = p.Signal(syscall.SIGTERM)
	}
}

// Name implements module.Module.
func (m *Module) Name() string { return "backup" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

func (m *Module) log() *slog.Logger { return m.d.Log }

// job is the state of the running or last export or restore.
type job struct {
	mu sync.Mutex
	v  api.BackupJob
}

func (j *job) set(f func(*api.BackupJob)) {
	j.mu.Lock()
	f(&j.v)
	j.mu.Unlock()
}

func (j *job) snapshot() api.BackupJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.v
}

func (j *job) step(text string) { j.set(func(v *api.BackupJob) { v.Step = &text }) }

func ptr[T any](v T) *T { return &v }

// begin starts a job, or fails with 409 when another one runs.
func (m *Module) begin(kind api.BackupJobKind) (*job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return nil, errBusy
	}
	j := &job{v: api.BackupJob{Kind: &kind, State: api.Running, StartedAt: ptr(m.now().UTC()), Step: ptr("准备中")}}
	m.cur, m.busy = j, true
	m.d.Bus.Publish("backup.job", j.v)
	return j, nil
}

// end finishes a job with the result of its work and remembers it.
func (m *Module) end(j *job, err error) {
	// The job reads as finished and a new one may start at the same moment:
	// a client that sees "done" can start the next job right away.
	m.mu.Lock()
	j.set(func(v *api.BackupJob) {
		v.FinishedAt = ptr(m.now().UTC())
		if err != nil {
			v.State, v.Error = api.Failed, ptr(err.Error())
			v.Step = nil
		} else {
			v.State, v.Error = api.Done, nil
			v.Step = nil
		}
	})
	m.busy = false
	m.mu.Unlock()
	final := j.snapshot()
	if perr := m.d.Settings.Set(context.Background(), keyJob, final); perr != nil {
		m.log().Warn("backup: save job state", "error", perr)
	}
	m.d.Bus.Publish("backup.job", final)
}

// GetBackupJob is GET /backups/job.
func (m *Module) GetBackupJob(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	cur := m.cur
	m.mu.Unlock()
	if cur != nil {
		httpx.JSON(w, http.StatusOK, cur.snapshot())
		return
	}
	last := api.BackupJob{State: api.Idle}
	if err := m.d.Settings.Get(r.Context(), keyJob, &last); err != nil && !errors.Is(err, settings.ErrNotSet) {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, last)
}

// ExportBackup is POST /backups/export.
func (m *Module) ExportBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	j, err := m.begin(api.BackupJobKindExport)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bg := context.WithoutCancel(ctx)
	go func() {
		name, size, err := m.create(bg, kindManual, m.local, "", false, j)
		if err == nil {
			j.set(func(v *api.BackupJob) { v.BackupId = &name })
			m.pruneLocal(bg)
			m.d.Bus.Publish("backup.created", map[string]any{"id": name})
		}
		m.d.Audit.Record(bg, "backup.export", name, map[string]any{"bytes": size}, err)
		m.end(j, err)
	}()
	httpx.JSON(w, http.StatusAccepted, j.snapshot())
}

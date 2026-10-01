import { useEffect, useRef, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router";
import {
  Archive,
  Download,
  PackageOpen,
  Play,
  RotateCcw,
  Trash2,
  Upload,
} from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import Switch from "../../components/ui/Switch";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatBytes, formatDate, relativeTime } from "../../lib/time";
import {
  downloadUrl,
  useBackupJob,
  useBackups,
  useBackupSettings,
  useDeleteBackup,
  useExportBackup,
  useRestoreBackup,
  useRunBackupNow,
  useSaveBackupSettings,
  useStartGdriveAuth,
  useTestBackupTarget,
  useUploadBackup,
  type Backup,
  type BackupJob,
  type BackupSettings,
  type BackupSettingsInput,
  type BackupTargetTest,
} from "./api";
import S3Fields, { emptyS3, s3Input, type S3Form } from "../storage/S3Fields";
import {
  GdriveFields,
  WebdavFields,
  gdriveForm,
  gdriveInput,
  webdavForm,
  webdavInput,
  type GdriveForm,
  type WebdavForm,
} from "./TargetFields";
import "./i18n";
import "./backup.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 设置 → 备份（B25）：导出、上传、恢复、自动备份到 S3。 */
export default function BackupTab() {
  const t = useT();
  const list = useBackups();
  if (list.isPending) return <Loading />;
  if (list.isError)
    return isNotLive(list.error) ? (
      <NotLive name={t("Backup")} />
    ) : (
      <ErrorState error={list.error} onRetry={() => list.refetch()} />
    );
  return (
    <div className="settings-grid">
      <ExportCard />
      <AutoCard />
      <ListCard items={list.data.items} />
    </div>
  );
}

function JobStatus({ job }: { job: BackupJob }) {
  const t = useT();
  const percent =
    job.totalBytes && job.doneBytes
      ? Math.round((job.doneBytes / job.totalBytes) * 100)
      : null;
  if (job.state === "idle") return null;
  return (
    <div className={`backup-job ${job.state}`} role="status">
      <strong>
        {job.state === "running"
          ? job.step || t("Working on it")
          : job.state === "done"
            ? t(job.kind === "restore" ? "Restored" : "Backup ready")
            : t(job.kind === "restore" ? "Restore failed" : "Backup failed")}
      </strong>
      {job.state === "running" && percent !== null && (
        <div className="backup-bar">
          <i style={{ width: `${percent}%` }} />
        </div>
      )}
      {job.error && <p className="xc-error-text">{job.error}</p>}
      {!!job.secretsUnreadable?.length && (
        <p className="backup-warn">
          {t("These settings could not be decrypted. Enter them again:")}{" "}
          {job.secretsUnreadable.join("、")}
        </p>
      )}
      {job.state === "done" && job.kind !== "restore" && job.backupId && (
        <a className="xc-btn small" href={downloadUrl(job.backupId)}>
          <Download size={13} /> {t("Download")}
        </a>
      )}
    </div>
  );
}

function ExportCard() {
  const t = useT();
  const job = useBackupJob();
  const exporter = useExportBackup();
  const upload = useUploadBackup();
  const fileRef = useRef<HTMLInputElement>(null);
  const running = job.data?.state === "running";
  // 恢复完成后重新加载页面，拿到恢复后的数据。
  // 只在这个页面亲眼看到恢复在进行时才刷新，否则重新登录后打开页面会一直刷新。
  const sawRestore = useRef(false);
  const restoreState = job.data?.kind === "restore" ? job.data.state : null;
  useEffect(() => {
    if (restoreState === "running") sawRestore.current = true;
  }, [restoreState]);
  const finishedRestore = restoreState === "done";
  useEffect(() => {
    if (!finishedRestore || !sawRestore.current) return;
    toast(t("Restored. Reloading."));
    const timer = setTimeout(() => window.location.reload(), 1500);
    return () => clearTimeout(timer);
  }, [finishedRestore, t]);
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Export and restore")}</h2>
      </div>
      <p className="backup-note">
        导出包里有数据库和全部文件。令牌和密钥是加密的，恢复时要用同一个主密钥（部署目录
        .env 里的 XC_MASTER_KEY），请把它单独保存好。
      </p>
      {job.data && <JobStatus job={job.data} />}
      <div className="backup-actions">
        <input
          ref={fileRef}
          type="file"
          accept=".tar.gz,.tgz,application/gzip"
          hidden
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = "";
            if (!file) return;
            upload.mutate(file, {
              onSuccess: () => toast(t("Uploaded. Restore it from the list.")),
              onError: fail,
            });
          }}
        />
        <button
          className="xc-btn"
          disabled={upload.isPending || running}
          onClick={() => fileRef.current?.click()}
        >
          <Upload size={14} />{" "}
          {upload.isPending ? t("Uploading") : t("Upload a backup")}
        </button>
        <button
          className="xc-btn primary"
          disabled={exporter.isPending || running}
          onClick={() => exporter.mutate(undefined, { onError: fail })}
        >
          <Archive size={14} /> {t("Export all data")}
        </button>
      </div>
    </section>
  );
}

const weekdays = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
];

const targetHints: Record<BackupSettings["target"], string> = {
  storage: "放在那个桶的 backups/ 目录下。超过保留份数时删最旧的。",
  custom: "超过保留份数时删最旧的。",
  webdav: "备份放在下面的目录里。超过保留份数时删最旧的。",
  gdrive: "备份放在网盘的一个文件夹里。超过保留份数时删最旧的。",
};

/** Google 授权完跳回设置页时，地址里带着结果。提示一次，然后去掉参数。 */
function useGdriveReturn() {
  const [params, setParams] = useSearchParams();
  const result = params.get("gdrive");
  const message = params.get("message");
  useEffect(() => {
    if (!result) return;
    if (result === "ok") toast("Google Drive 已授权");
    else
      toast({
        message: `Google Drive 授权失败：${message || "未知原因"}`,
        tone: "error",
      });
    setParams(
      (p) => {
        p.delete("gdrive");
        p.delete("message");
        return p;
      },
      { replace: true },
    );
  }, [result, message, setParams]);
}

function AutoCard() {
  const t = useT();
  const language = useLanguage();
  const settings = useBackupSettings();
  const save = useSaveBackupSettings();
  const run = useRunBackupNow();
  const test = useTestBackupTarget();
  const startAuth = useStartGdriveAuth();
  const [form, setForm] = useState<BackupSettings | null>(null);
  const [s3, setS3] = useState<S3Form>(() => emptyS3(undefined, "backups"));
  const [webdav, setWebdav] = useState<WebdavForm>(() => webdavForm());
  const [gdrive, setGdrive] = useState<GdriveForm>(() => gdriveForm());
  const [tested, setTested] = useState<BackupTargetTest | null>(null);
  useGdriveReturn();
  useEffect(() => {
    if (!settings.data) return;
    setForm(settings.data);
    setS3(emptyS3(settings.data.s3, "backups"));
    setWebdav(webdavForm(settings.data.webdav));
    setGdrive(gdriveForm(settings.data.gdrive));
  }, [settings.data]);
  if (settings.isPending) return <Loading />;
  if (settings.isError || !form)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );
  const set = <K extends keyof BackupSettings>(k: K, v: BackupSettings[K]) =>
    setForm((f) => (f ? { ...f, [k]: v } : f));
  const targetInput = (): BackupSettingsInput => ({
    target: form.target,
    ...(form.target === "custom" ? { s3: s3Input(s3) } : {}),
    ...(form.target === "webdav" ? { webdav: webdavInput(webdav) } : {}),
    ...(form.target === "gdrive" ? { gdrive: gdriveInput(gdrive) } : {}),
  });
  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      {
        enabled: form.enabled,
        frequency: form.frequency,
        time: form.time,
        weekday: form.weekday,
        keep: form.keep,
        ...targetInput(),
      },
      { onSuccess: () => toast(t("Saved")), onError: fail },
    );
  };
  const onTest = () => {
    setTested(null);
    test.mutate(targetInput(), { onSuccess: setTested, onError: fail });
  };
  // 先只保存客户端（不改备份位置和开关），再跳到 Google 授权页。
  const onAuthorize = () => {
    save.mutate(
      { gdrive: gdriveInput(gdrive) },
      {
        onSuccess: () =>
          startAuth.mutate(undefined, {
            onSuccess: (r) => {
              window.location.href = r.url;
            },
            onError: fail,
          }),
        onError: fail,
      },
    );
  };
  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>{t("Automatic backup")}</h2>
        <Switch
          checked={form.enabled}
          onChange={(v) => set("enabled", v)}
          label={t("Automatic backup")}
        />
      </div>
      <div className="backup-row">
        <label className="xc-field">
          <span>{t("How often")}</span>
          <select
            className="xc-select"
            value={form.frequency}
            onChange={(e) =>
              set("frequency", e.target.value as BackupSettings["frequency"])
            }
          >
            <option value="daily">{t("Every day")}</option>
            <option value="weekly">{t("Every week")}</option>
          </select>
        </label>
        {form.frequency === "weekly" && (
          <label className="xc-field">
            <span>{t("Day of week")}</span>
            <select
              className="xc-select"
              value={form.weekday}
              onChange={(e) => set("weekday", Number(e.target.value))}
            >
              {weekdays.map((d, i) => (
                <option key={d} value={i}>
                  {t(d)}
                </option>
              ))}
            </select>
          </label>
        )}
        <label className="xc-field">
          <span>{t("Time")}</span>
          <input
            className="xc-input"
            type="time"
            value={form.time}
            onChange={(e) => set("time", e.target.value)}
          />
        </label>
        <label className="xc-field">
          <span>{t("Keep")}</span>
          <input
            className="xc-input"
            inputMode="numeric"
            value={String(form.keep)}
            onChange={(e) =>
              set("keep", Number(e.target.value.replace(/\D/g, "")) || 1)
            }
          />
        </label>
      </div>
      <label className="xc-field">
        <span>{t("Where to")}</span>
        <select
          className="xc-select"
          value={form.target}
          onChange={(e) => {
            set("target", e.target.value as BackupSettings["target"]);
            setTested(null);
          }}
        >
          <option value="storage">{t("The S3 in Storage settings")}</option>
          <option value="custom">{t("Another S3")}</option>
          <option value="webdav">WebDAV（坚果云、Nextcloud、alist 等）</option>
          <option value="gdrive">Google Drive</option>
        </select>
        <small>{targetHints[form.target]}</small>
      </label>
      {form.target === "custom" && (
        <S3Fields value={s3} onChange={setS3} hasSecret={form.s3?.hasSecret} />
      )}
      {form.target === "webdav" && (
        <WebdavFields
          value={webdav}
          onChange={setWebdav}
          passwordSet={form.webdav?.passwordSet}
        />
      )}
      {form.target === "gdrive" && (
        <GdriveFields
          value={gdrive}
          onChange={setGdrive}
          saved={settings.data?.gdrive}
          authorizing={save.isPending || startAuth.isPending}
          onAuthorize={onAuthorize}
        />
      )}
      {tested && (
        <p
          className={`backup-test ${tested.ok ? "ok" : "failed"}`}
          role="status"
        >
          {tested.message}
        </p>
      )}
      {form.nextRunAt && form.enabled && (
        <p className="backup-note">
          {t("Next backup")}：{formatDate(form.nextRunAt, language)} {form.time}
        </p>
      )}
      {form.lastRuns.length > 0 && (
        <ul className="backup-runs">
          {form.lastRuns.slice(0, 5).map((r) => (
            <li key={r.at}>
              <span className={`xc-dot ${r.ok ? "ok" : "danger"}`} />
              <span>{relativeTime(r.at, language)}</span>
              <small className="xc-muted">
                {r.ok ? (r.sizeBytes ? formatBytes(r.sizeBytes) : "") : r.error}
              </small>
            </li>
          ))}
        </ul>
      )}
      <div className="backup-actions">
        <button
          type="button"
          className="xc-btn"
          disabled={run.isPending}
          onClick={() =>
            run.mutate(undefined, {
              onSuccess: () => toast(t("Backup started")),
              onError: fail,
            })
          }
        >
          <Play size={13} /> {t("Back up now")}
        </button>
        <button
          type="button"
          className="xc-btn"
          disabled={test.isPending}
          onClick={onTest}
        >
          {test.isPending ? t("Testing") : t("Test connection")}
        </button>
        <button className="xc-btn primary" disabled={save.isPending}>
          {t("Save")}
        </button>
      </div>
    </form>
  );
}

const locationLabels: Partial<Record<Backup["location"], string>> = {
  s3: "S3",
  webdav: "WebDAV",
  gdrive: "Google Drive",
};

const kindLabels: Record<Backup["kind"], string> = {
  manual: "Manual",
  auto: "Automatic",
  "pre-restore": "Before restore",
  uploaded: "Uploaded backup",
};

function ListCard({ items }: { items: Backup[] }) {
  const t = useT();
  const language = useLanguage();
  const restore = useRestoreBackup();
  const remove = useDeleteBackup();
  const onRestore = async (b: Backup) => {
    const ok = await confirmAction({
      title: `${t("Restore from")} ${b.name}？`,
      description:
        "现在的全部数据和文件会被这份备份替换。恢复前会自动再备份一次现在的数据。",
      confirmLabel: t("Restore"),
      typeToConfirm: "恢复",
    });
    if (!ok) return;
    restore.mutate(b.id, {
      onSuccess: () => toast(t("Restoring")),
      onError: fail,
    });
  };
  const onDelete = async (b: Backup) => {
    if (!(await confirmAction({ title: `${t("Delete")} ${b.name}？` }))) return;
    remove.mutate(b.id, { onError: fail });
  };
  return (
    <section className="xc-card backup-list-card">
      <div className="xc-card-head">
        <h2>{t("Backups")}</h2>
        <span className="xc-muted">{items.length}</span>
      </div>
      {items.length === 0 ? (
        <EmptyState
          title={t("No backups yet")}
          icon={<PackageOpen size={26} />}
        >
          <span>{t("Export once, or turn on automatic backup.")}</span>
        </EmptyState>
      ) : (
        <ul className="backup-list">
          {items.map((b) => (
            <li key={b.id}>
              <div className="backup-list-main">
                <strong title={b.name}>{b.name}</strong>
                <small className="xc-muted">
                  {formatDate(b.createdAt, language)} ·{" "}
                  {formatBytes(b.sizeBytes)}
                  {b.version ? ` · ${b.version}` : ""}
                </small>
              </div>
              <span className="xc-badge">
                {locationLabels[b.location] ?? t("On this server")}
              </span>
              <span className="xc-badge">{t(kindLabels[b.kind])}</span>
              <MoreMenu
                label={`${t("More")}：${b.name}`}
                title={b.name}
                items={[
                  {
                    key: "download",
                    label: t("Download"),
                    icon: <Download size={14} />,
                    onSelect: () => {
                      window.location.href = downloadUrl(b.id);
                    },
                  },
                  {
                    key: "restore",
                    label: t("Restore"),
                    icon: <RotateCcw size={14} />,
                    onSelect: () => onRestore(b),
                  },
                  {
                    key: "delete",
                    label: t("Delete"),
                    icon: <Trash2 size={14} />,
                    danger: true,
                    onSelect: () => onDelete(b),
                  },
                ]}
              />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

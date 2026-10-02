import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link } from "react-router";
import { Segmented } from "../../components/ui/Toolbar";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import {
  Archive,
  ChevronDown,
  ChevronRight,
  Download,
  ShieldCheck,
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
import {
  formatBytes,
  formatDate,
  formatTime,
  relativeTime,
} from "../../lib/time";
import {
  downloadUrl,
  useBackupJob,
  useBackups,
  useBackupSettings,
  useCheckBackup,
  useDeleteBackup,
  useRestoreSnapshot,
  useSnapshotChanges,
  useSnapshots,
  useExportBackup,
  useRestoreBackup,
  useRunBackupNow,
  useSaveBackupSettings,
  useTestBackupTarget,
  useUploadBackup,
  type Backup,
  type BackupJob,
  type BackupRetention,
  type BackupSettings,
  type BackupSnapshot,
  type BackupSettingsInput,
  type BackupTargetTest,
} from "./api";
import S3Fields, { emptyS3, s3Input, type S3Form } from "../storage/S3Fields";
import { useRemotes } from "../storage/api";
import "./i18n";
import "./backup.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 设置 → 备份（B25）：导出、上传、恢复、自动备份到 S3。 */
export default function BackupTab() {
  const t = useT();
  const list = useBackups();
  const settings = useBackupSettings();
  const incremental = settings.data?.mode === "incremental";
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
      {incremental && <SnapshotsCard />}
      <ListCard items={list.data.items} />
    </div>
  );
}

const doneLabels: Record<NonNullable<BackupJob["kind"]>, string> = {
  export: "Backup ready",
  auto: "Backup ready",
  restore: "Restored",
  check: "Check passed",
  prune: "Old snapshots cleaned up",
};
const failLabels: Record<NonNullable<BackupJob["kind"]>, string> = {
  export: "Backup failed",
  auto: "Backup failed",
  restore: "Restore failed",
  check: "Check failed",
  prune: "Cleanup failed",
};

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
            ? t(doneLabels[job.kind ?? "export"])
            : t(failLabels[job.kind ?? "export"])}
      </strong>
      {job.state === "running" && percent !== null && (
        <div className="backup-bar">
          <i style={{ width: `${percent}%` }} />
        </div>
      )}
      {job.error && <p className="xc-error-text">{job.error}</p>}
      {job.warning && <p className="backup-warn">{job.warning}</p>}
      {job.state === "done" && job.check && (
        <p className="backup-note">
          {t("Checked {s} snapshots and {b} blocks. All are there.")
            .replace("{s}", String(job.check.snapshots))
            .replace("{b}", String(job.check.blocks))}
        </p>
      )}
      {job.prune && job.prune.snapshots > 0 && (
        <p className="backup-note">
          {t("Removed {s} old snapshots and freed {size}.")
            .replace("{s}", String(job.prune.snapshots))
            .replace("{size}", formatBytes(job.prune.bytes))}
        </p>
      )}
      {!!job.secretsUnreadable?.length && (
        <p className="backup-warn">
          {t("These settings could not be decrypted. Enter them again:")}{" "}
          {job.secretsUnreadable.join("、")}
        </p>
      )}
      {/* 只有完整包能下载。增量快照的编号不是备份文件（B81） */}
      {job.state === "done" && job.kind === "export" && job.backupId && (
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
  remote: "备份放在网盘账号的一个文件夹里。超过保留份数时删最旧的。",
  webdav: "",
  gdrive: "",
};

/** “备份到”下拉框的值：storage、custom，或者 remote:<账号 id>。 */
function targetValue(s: BackupSettings): string {
  if (s.target === "remote" && s.remoteId) return `remote:${s.remoteId}`;
  return s.target === "custom" ? "custom" : "storage";
}

function AutoCard() {
  const t = useT();
  const language = useLanguage();
  const settings = useBackupSettings();
  const save = useSaveBackupSettings();
  const run = useRunBackupNow();
  const test = useTestBackupTarget();
  const remotes = useRemotes();
  const [form, setForm] = useState<BackupSettings | null>(null);
  const [s3, setS3] = useState<S3Form>(() => emptyS3(undefined, "backups"));
  const [folder, setFolder] = useState("x-console-backups");
  const [folderName, setFolderName] = useState("X Console 备份");
  const [tested, setTested] = useState<BackupTargetTest | null>(null);
  useEffect(() => {
    if (!settings.data) return;
    setForm(settings.data);
    setS3(emptyS3(settings.data.s3, "backups"));
    setFolder(settings.data.webdav?.folder ?? "x-console-backups");
    setFolderName(settings.data.gdrive?.folderName ?? "X Console 备份");
  }, [settings.data]);
  if (settings.isPending) return <Loading />;
  if (settings.isError || !form)
    return (
      <ErrorState error={settings.error} onRetry={() => settings.refetch()} />
    );
  const set = <K extends keyof BackupSettings>(k: K, v: BackupSettings[K]) =>
    setForm((f) => (f ? { ...f, [k]: v } : f));
  const accounts = remotes.data?.items ?? [];
  const account =
    form.target === "remote"
      ? accounts.find((a) => a.id === form.remoteId)
      : undefined;
  const pickTarget = (value: string) => {
    setTested(null);
    setForm((f) => {
      if (!f) return f;
      if (value.startsWith("remote:"))
        return { ...f, target: "remote", remoteId: Number(value.slice(7)) };
      return { ...f, target: value === "custom" ? "custom" : "storage" };
    });
  };
  const targetInput = (): BackupSettingsInput => {
    if (form.target === "custom") return { target: "custom", s3: s3Input(s3) };
    if (form.target !== "remote") return { target: "storage" };
    return {
      target: "remote",
      remoteId: form.remoteId,
      ...(account?.kind === "gdrive"
        ? { gdrive: { folderName: folderName.trim() } }
        : { webdav: { folder: folder.trim().replace(/^\/+|\/+$/g, "") } }),
    };
  };
  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      {
        enabled: form.enabled,
        mode: form.mode,
        retention: form.retention,
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
      <div className="xc-field">
        <span>{t("Backup method")}</span>
        <Segmented
          label={t("Backup method")}
          value={form.mode}
          onChange={(v) => set("mode", v)}
          options={[
            { value: "incremental", label: t("Incremental") },
            { value: "full", label: t("Full package") },
          ]}
        />
        <small>
          {form.mode === "incremental"
            ? "只传变化的部分，每次是一个快照，能回到任意一次。第一次会传全部。"
            : "每次打一个完整的压缩包。"}
        </small>
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
        {form.mode === "full" && (
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
        )}
      </div>
      {form.mode === "incremental" && (
        <RetentionFields
          value={form.retention}
          onChange={(v) => set("retention", v)}
        />
      )}
      <label className="xc-field">
        <span>{t("Where to")}</span>
        <select
          className="xc-select"
          value={targetValue(form)}
          onChange={(e) => pickTarget(e.target.value)}
        >
          <option value="storage">{t("The S3 in Storage settings")}</option>
          <option value="custom">{t("Another S3")}</option>
          {accounts.map((a) => (
            <option key={a.id} value={`remote:${a.id}`}>
              {a.name}（{a.kind === "gdrive" ? "Google Drive" : "WebDAV"}）
            </option>
          ))}
        </select>
        <small>
          {targetHints[form.target]}
          {accounts.length === 0 && !remotes.isPending && (
            <>
              {" "}
              要备份到坚果云、Google Drive 这类网盘，先到{" "}
              <Link to="/settings/storage">设置 → 存储</Link> 添加网盘账号。
            </>
          )}
        </small>
      </label>
      {form.target === "custom" && (
        <S3Fields value={s3} onChange={setS3} hasSecret={form.s3?.hasSecret} />
      )}
      {account && !account.ready && (
        <p className="backup-warn">
          {account.kind === "gdrive"
            ? "这个账号还没授权，"
            : "这个账号还没填完整，"}
          到 <Link to="/settings/storage">设置 → 存储</Link> 处理。
        </p>
      )}
      {account?.kind === "webdav" && (
        <label className="xc-field">
          <span>{t("Folder")}</span>
          <input
            className="xc-input"
            value={folder}
            onChange={(e) => setFolder(e.target.value)}
            placeholder="x-console-backups"
            spellCheck={false}
          />
        </label>
      )}
      {account?.kind === "gdrive" && (
        <label className="xc-field">
          <span>{t("Folder name")}</span>
          <input
            className="xc-input"
            value={folderName}
            onChange={(e) => setFolderName(e.target.value)}
            placeholder="X Console 备份"
          />
          <small>放在网盘根目录下，没有就新建。</small>
        </label>
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

const retentionFields: { key: keyof BackupRetention; label: string }[] = [
  { key: "last", label: "Latest (times)" },
  { key: "daily", label: "Daily (days)" },
  { key: "weekly", label: "Weekly (weeks)" },
  { key: "monthly", label: "Monthly (months)" },
];

/** 保留规则（B81）：满足任意一条就保留。 */
function RetentionFields({
  value,
  onChange,
}: {
  value: BackupRetention;
  onChange: (v: BackupRetention) => void;
}) {
  const t = useT();
  return (
    <div className="xc-field">
      <span>{t("Keep snapshots")}</span>
      <div className="backup-row backup-retention">
        {retentionFields.map((f) => (
          <label key={f.key} className="xc-field">
            <small>{t(f.label)}</small>
            <input
              className="xc-input"
              inputMode="numeric"
              value={String(value[f.key])}
              onChange={(e) =>
                onChange({
                  ...value,
                  [f.key]: Number(e.target.value.replace(/\D/g, "")) || 0,
                })
              }
            />
          </label>
        ))}
      </div>
      <small>满足任意一条就保留。过期快照独有的数据会从备份里删掉。</small>
    </div>
  );
}

/** 增量备份的快照列表（B81）：概要、检查、每次的变化、恢复到某一次。 */
function SnapshotsCard() {
  const t = useT();
  const language = useLanguage();
  const snapshots = useSnapshots(true);
  const check = useCheckBackup();
  const restore = useRestoreSnapshot();
  const job = useBackupJob();
  const [open, setOpen] = useState<string | null>(null);
  const busy = job.data?.state === "running";
  const onRestore = async (s: BackupSnapshot) => {
    const ok = await confirmAction({
      title: `${t("Restore to")} ${formatDate(s.createdAt, language)} ${formatTime(s.createdAt, language)}？`,
      description:
        "现在的全部数据和文件会换成这个快照里的。恢复前会自动保存一份现在的完整包。",
      confirmLabel: t("Restore"),
      typeToConfirm: "恢复",
    });
    if (!ok) return;
    restore.mutate(s.id, {
      onSuccess: () => toast(t("Restoring")),
      onError: fail,
    });
  };
  if (snapshots.isPending)
    return (
      <section className="xc-card">
        <Loading />
      </section>
    );
  if (snapshots.isError)
    return (
      <section className="xc-card">
        <ErrorState
          error={snapshots.error}
          onRetry={() => snapshots.refetch()}
        />
      </section>
    );
  const { items, stats } = snapshots.data;
  const saved = Math.max(0, stats.logicalBytes - stats.sizeBytes);
  return (
    <section className="xc-card backup-snapshots">
      <div className="xc-card-head">
        <h2>{t("Snapshots")}</h2>
        <button
          type="button"
          className="xc-btn small"
          disabled={busy || check.isPending || items.length === 0}
          onClick={() =>
            check.mutate(undefined, {
              onSuccess: () => toast(t("Checking backup")),
              onError: fail,
            })
          }
        >
          <ShieldCheck size={14} /> {t("Check backup")}
        </button>
      </div>
      <StatStrip label={t("Snapshots")}>
        <StatCard label={t("Snapshots")} value={stats.snapshots} />
        <StatCard
          label={t("Backup size")}
          value={formatBytes(stats.sizeBytes)}
        />
        <StatCard
          label={t("Last backup")}
          value={
            stats.lastSnapshotAt
              ? relativeTime(stats.lastSnapshotAt, language)
              : "—"
          }
        />
        <StatCard
          label={t("Saved by dedup")}
          value={formatBytes(saved)}
          foot={t("vs. full packages")}
        />
      </StatStrip>
      {items.length === 0 ? (
        <EmptyState
          title={t("No snapshots yet")}
          icon={<PackageOpen size={26} />}
        >
          <span>
            {t("Click Back up now, or wait for the next automatic backup.")}
          </span>
        </EmptyState>
      ) : (
        <ul className="backup-list backup-snapshot-list">
          {items.map((s) => (
            <li key={s.id}>
              <div className="backup-snapshot-row">
                <button
                  type="button"
                  className="xc-btn ghost small"
                  aria-expanded={open === s.id}
                  aria-label={t("Show snapshot changes")}
                  title={t("Show snapshot changes")}
                  onClick={() => setOpen(open === s.id ? null : s.id)}
                >
                  {open === s.id ? (
                    <ChevronDown size={14} />
                  ) : (
                    <ChevronRight size={14} />
                  )}
                </button>
                <div className="backup-list-main">
                  <strong>
                    {formatDate(s.createdAt, language)}{" "}
                    {formatTime(s.createdAt, language)}
                  </strong>
                  <small className="xc-muted">
                    <span className="backup-added">+{s.added}</span>{" "}
                    <span className="backup-modified">~{s.modified}</span>{" "}
                    <span className="backup-deleted">-{s.deleted}</span> ·{" "}
                    {t("Uploaded this time")} {formatBytes(s.uploadedBytes)} ·{" "}
                    {formatBytes(s.sizeBytes)}
                  </small>
                </div>
                <MoreMenu
                  label={`${t("More")}：${s.id}`}
                  items={[
                    {
                      key: "restore",
                      label: t("Restore to here"),
                      icon: <RotateCcw size={14} />,
                      danger: true,
                      onSelect: () => void onRestore(s),
                    },
                  ]}
                />
              </div>
              {open === s.id && <SnapshotChanges id={s.id} />}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

const changeLabels = {
  added: "File added",
  modified: "File changed",
  deleted: "File deleted",
} as const;

function SnapshotChanges({ id }: { id: string }) {
  const t = useT();
  const changes = useSnapshotChanges(id);
  if (changes.isPending) return <Loading />;
  if (changes.isError)
    return (
      <ErrorState error={changes.error} onRetry={() => changes.refetch()} />
    );
  if (changes.data.items.length === 0)
    return (
      <p className="xc-muted backup-changes-empty">{t("No file changes")}</p>
    );
  return (
    <div className="backup-changes">
      {changes.data.items.map((c) => (
        <div key={`${c.kind}:${c.path}`} className={`backup-change ${c.kind}`}>
          <span>{t(changeLabels[c.kind])}</span>
          <span className="xc-mono" title={c.path}>
            {c.path}
          </span>
        </div>
      ))}
      {changes.data.truncated && (
        <small className="xc-muted">{t("Only the first 200 are shown.")}</small>
      )}
    </div>
  );
}

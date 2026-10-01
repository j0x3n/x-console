import { useEffect, useState, type FormEvent } from "react";
import { ArrowLeftRight, HardDrive, PlugZap, Square } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { formatBytes, relativeTime } from "../../lib/time";
import S3SettingsTab from "../drive/S3SettingsTab";
import {
  moduleLabels,
  useCancelSwitch,
  useSaveCache,
  useSaveStorageS3,
  useStorage,
  useSwitchStorage,
  useTestStorageS3,
  type StorageStatus,
  type TestResult,
} from "./api";
import RemotesCard from "./RemotesCard";
import S3Fields, { emptyS3, s3Input, type S3Form } from "./S3Fields";
import "./i18n";
import "./storage.css";

/*
 * 设置 → 存储（B24）：整站文件放在本机磁盘还是 S3。
 * 网盘账号（B69）也在这里管，备份和云盘页共用。
 * 后端还没上线时，显示云盘原来的 S3 同步。
 */
export default function StorageTab() {
  const t = useT();
  const storage = useStorage();
  if (storage.isPending) return <Loading />;
  if (storage.isError) {
    if (!isNotLive(storage.error))
      return (
        <ErrorState error={storage.error} onRetry={() => storage.refetch()} />
      );
    return (
      <>
        <p className="storage-legacy-note" role="status">
          {t("The new storage settings are not live yet.")}{" "}
          {t("Until then, the drive keeps using its S3 sync below.")}
        </p>
        <S3SettingsTab />
      </>
    );
  }
  return (
    <div className="settings-grid">
      <LocationCard status={storage.data} />
      <S3Card status={storage.data} />
      <RemotesCard />
      <UsageCard status={storage.data} />
    </div>
  );
}

function LocationCard({ status }: { status: StorageStatus }) {
  const t = useT();
  const language = useLanguage();
  const switcher = useSwitchStorage();
  const cancel = useCancelSwitch();
  const [deleteSource, setDeleteSource] = useState(false);
  const m = status.migration;
  const running = m.state === "running";
  const target = status.backend === "local" ? "s3" : "local";
  const percent =
    m.totalBytes > 0 ? Math.round((m.doneBytes / m.totalBytes) * 100) : 0;

  const start = async () => {
    if (target === "s3" && !status.s3?.bucket)
      return toast({ message: t("Set up S3 first"), tone: "error" });
    const ok = await confirmAction({
      title:
        target === "s3" ? "把全部文件搬到 S3？" : "把全部文件搬回本机磁盘？",
      description:
        "搬完并核对大小后才会切换。搬的过程中照常使用，新上传的文件也会一起搬。" +
        (deleteSource ? "切换成功后会删除原位置的文件。" : ""),
      confirmLabel:
        target === "s3" ? t("Switch to S3") : t("Switch to local disk"),
      danger: false,
    });
    if (!ok) return;
    switcher.mutate(
      { target, deleteSource },
      {
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };

  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Storage location")}</h2>
        <span className="xc-badge accent">
          {status.backend === "s3" ? "S3" : t("Local disk")}
        </span>
      </div>
      <p className="storage-note">
        云盘、笔记附件、项目图片、历史版本都存在这里。
        {status.backend === "local" ? (
          <>
            {" "}
            现在放在 <code>{status.localPath}</code>，备份这个目录就行。
          </>
        ) : (
          <>
            {" "}
            现在放在 S3 的 {status.s3?.bucket}/{status.s3?.prefix}。
          </>
        )}
      </p>
      {m.state !== "idle" && m.state !== "done" && (
        <div className="storage-migration" role="status">
          <div className="storage-migration-head">
            <strong>
              {running
                ? t("Moving files")
                : m.state === "canceled"
                  ? t("Moving stopped")
                  : t("Moving failed")}
            </strong>
            <span className="xc-muted">
              {m.doneFiles} / {m.totalFiles} · {formatBytes(m.doneBytes)} /{" "}
              {formatBytes(m.totalBytes)}
            </span>
          </div>
          <div className="storage-bar">
            <i style={{ width: `${percent}%` }} />
          </div>
          {m.startedAt && (
            <small className="xc-muted">
              {relativeTime(m.startedAt, language)}开始
            </small>
          )}
          {m.error && <p className="xc-error-text">{m.error}</p>}
        </div>
      )}
      {!running && (
        <label className="xc-check">
          <input
            type="checkbox"
            checked={deleteSource}
            onChange={(e) => setDeleteSource(e.target.checked)}
          />
          <span>{t("Delete the files in the old place after switching")}</span>
        </label>
      )}
      <div className="storage-actions">
        {running ? (
          <button
            className="xc-btn"
            disabled={cancel.isPending}
            onClick={() => cancel.mutate()}
          >
            <Square size={13} /> {t("Stop moving")}
          </button>
        ) : (
          <button
            className="xc-btn primary"
            disabled={switcher.isPending}
            onClick={start}
          >
            <ArrowLeftRight size={14} />{" "}
            {m.state === "failed" || m.state === "canceled"
              ? t("Try again")
              : target === "s3"
                ? t("Switch to S3")
                : t("Switch to local disk")}
          </button>
        )}
      </div>
    </section>
  );
}

function S3Card({ status }: { status: StorageStatus }) {
  const t = useT();
  const save = useSaveStorageS3();
  const test = useTestStorageS3();
  const [form, setForm] = useState<S3Form>(() => emptyS3(status.s3));
  const [result, setResult] = useState<TestResult | null>(null);
  useEffect(() => setForm(emptyS3(status.s3)), [status.s3]);
  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(s3Input(form), {
      onSuccess: () => toast(t("Saved")),
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });
  };
  const onTest = () => {
    setResult(null);
    test.mutate(s3Input(form), {
      onSuccess: setResult,
      onError: (err) => setResult({ ok: false, message: errorMessage(err) }),
    });
  };
  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>{t("S3 settings")}</h2>
      </div>
      <p className="storage-note">
        支持 AWS S3、Cloudflare R2、MinIO、阿里云 OSS 等兼容 S3
        的服务。整站备份（设置 → 备份）也可以用它。
      </p>
      <S3Fields
        value={form}
        onChange={setForm}
        hasSecret={status.s3?.hasSecret}
      />
      {result && (
        <p
          className={`storage-test ${result.ok ? "ok" : "fail"}`}
          role="status"
        >
          {result.message}
        </p>
      )}
      <div className="storage-actions">
        <button
          type="button"
          className="xc-btn"
          onClick={onTest}
          disabled={test.isPending}
        >
          <PlugZap size={14} /> {t("Test connection")}
        </button>
        <button className="xc-btn primary" disabled={save.isPending}>
          {t("Save")}
        </button>
      </div>
    </form>
  );
}

function UsageCard({ status }: { status: StorageStatus }) {
  const t = useT();
  const saveCache = useSaveCache();
  const [cacheGB, setCacheGB] = useState(
    String(Math.round(status.cacheLimitBytes / 1024 ** 3)),
  );
  const total = status.usage.reduce((sum, u) => sum + u.bytes, 0);
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Usage")}</h2>
        <span className="xc-muted">{formatBytes(total)}</span>
      </div>
      <ul className="storage-usage">
        {status.usage.map((u) => (
          <li key={u.module}>
            <HardDrive size={14} />
            <span>{t(moduleLabels[u.module] ?? u.module)}</span>
            <small className="xc-muted">
              {u.files} {t("files")}
            </small>
            <strong>{formatBytes(u.bytes)}</strong>
          </li>
        ))}
      </ul>
      {status.backend === "s3" && (
        <form
          className="storage-cache"
          onSubmit={(e) => {
            e.preventDefault();
            const gb = Number(cacheGB);
            if (!Number.isFinite(gb) || gb < 0) return;
            saveCache.mutate(Math.round(gb * 1024 ** 3), {
              onSuccess: () => toast(t("Saved")),
              onError: (err) =>
                toast({ message: errorMessage(err), tone: "error" }),
            });
          }}
        >
          <label className="xc-field">
            <span>
              {t("Local cache")} · {formatBytes(status.cacheBytes)}
            </span>
            <div className="storage-cache-row">
              <input
                className="xc-input"
                inputMode="decimal"
                aria-label={t("Cache limit (GB)")}
                value={cacheGB}
                onChange={(e) => setCacheGB(e.target.value)}
              />
              <span className="xc-muted">GB</span>
              <button className="xc-btn" disabled={saveCache.isPending}>
                {t("Save")}
              </button>
            </div>
            <small>
              缩略图和最近打开的文件留在本机，打开更快。填 0 表示不缓存。
            </small>
          </label>
        </form>
      )}
    </section>
  );
}

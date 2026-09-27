import { useEffect, useState, type FormEvent } from "react";
import { PlugZap, RefreshCw } from "lucide-react";
import { errorMessage } from "../../api/client";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  isNotLive,
  useS3Config,
  useS3Status,
  useSaveS3Config,
  useSyncS3,
  useTestS3,
  type S3Config,
  type S3ConfigInput,
} from "./api";
import "./i18n";
import "./drive.css";

/** 设置 → 云盘同步：S3 兼容存储的配置、测试、立即同步。 */
export default function S3SettingsTab() {
  const config = useS3Config();
  if (config.isPending) return <Loading />;
  if (config.isError)
    return isNotLive(config.error) ? (
      <EmptyState title="云盘同步还没上线" />
    ) : (
      <ErrorState error={config.error} onRetry={() => config.refetch()} />
    );
  return (
    <div className="settings-grid">
      <S3Form initial={config.data} />
      <S3StatusCard />
    </div>
  );
}

type Form = Omit<S3Config, "hasSecret"> & { secretAccessKey: string };

function toForm(c: S3Config): Form {
  const { hasSecret: _hasSecret, ...rest } = c;
  return { ...rest, secretAccessKey: "" };
}

/** 只把要保存的字段发出去。Secret 留空表示不改。 */
export function toInput(form: Form): S3ConfigInput {
  const { secretAccessKey, ...rest } = form;
  const out: S3ConfigInput = {
    ...rest,
    endpoint: rest.endpoint.trim(),
    region: rest.region.trim(),
    bucket: rest.bucket.trim(),
    prefix: rest.prefix.trim().replace(/^\/+|\/+$/g, ""),
    accessKeyId: rest.accessKeyId.trim(),
  };
  if (secretAccessKey.trim()) out.secretAccessKey = secretAccessKey.trim();
  return out;
}

function S3Form({ initial }: { initial: S3Config }) {
  const t = useT();
  const save = useSaveS3Config();
  const test = useTestS3();
  const [form, setForm] = useState<Form>(() => toForm(initial));
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(
    null,
  );
  useEffect(() => setForm(toForm(initial)), [initial]);
  const set = <K extends keyof Form>(key: K, value: Form[K]) =>
    setForm((f) => ({ ...f, [key]: value }));

  const onSave = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(toInput(form), {
      onSuccess: () => toast(t("Saved")),
      onError: (error) =>
        toast({ message: errorMessage(error), tone: "error" }),
    });
  };
  const onTest = () => {
    setResult(null);
    test.mutate(toInput(form), {
      onSuccess: setResult,
      onError: (error) =>
        setResult({ ok: false, message: errorMessage(error) }),
    });
  };

  const text = (
    key: "endpoint" | "region" | "bucket" | "prefix" | "accessKeyId",
    label: string,
    placeholder: string,
    hint?: string,
  ) => (
    <label className="xc-field">
      <span>{label}</span>
      <input
        className="xc-input"
        value={form[key]}
        onChange={(e) => set(key, e.target.value)}
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
      />
      {hint && <small>{hint}</small>}
    </label>
  );

  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>{t("S3 storage")}</h2>
      </div>
      <p className="drive-muted drive-card-note">
        面板里的文件单向备份到 S3。支持 AWS S3、Cloudflare R2、MinIO、阿里云 OSS
        等兼容存储。
      </p>
      <label className="drive-check">
        <input
          type="checkbox"
          checked={form.enabled}
          onChange={(e) => set("enabled", e.target.checked)}
        />
        <span>{t("Sync to S3")}</span>
      </label>
      {text("endpoint", t("Endpoint"), "https://s3.amazonaws.com")}
      {text("region", t("Region"), "us-east-1", "R2 填 auto。")}
      {text("bucket", t("Bucket"), "my-backup")}
      {text(
        "prefix",
        t("Path prefix"),
        "x-console",
        "文件放在桶里的这个目录下，目录结构和面板一样。",
      )}
      {text("accessKeyId", "Access Key ID", "")}
      <label className="xc-field">
        <span>Secret Access Key</span>
        <input
          className="xc-input"
          type="password"
          value={form.secretAccessKey}
          onChange={(e) => set("secretAccessKey", e.target.value)}
          placeholder={initial.hasSecret ? t("leave empty to keep") : ""}
          autoComplete="new-password"
        />
      </label>
      <label className="drive-check">
        <input
          type="checkbox"
          checked={form.pathStyle}
          onChange={(e) => set("pathStyle", e.target.checked)}
        />
        <span>{t("Path-style URLs")}</span>
      </label>
      <small className="drive-muted drive-check-hint">MinIO 一般要勾上。</small>
      <label className="drive-check">
        <input
          type="checkbox"
          checked={form.includeHidden}
          onChange={(e) => set("includeHidden", e.target.checked)}
        />
        <span>{t("Also sync hidden files")}</span>
      </label>
      <small className="drive-muted drive-check-hint">
        放在前缀下的 .hidden 目录里。桶的权限要自己管好。
      </small>
      {result && (
        <p className={`drive-test ${result.ok ? "ok" : "fail"}`} role="status">
          {result.message}
        </p>
      )}
      <div className="drive-form-actions">
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

const STATE_LABELS = {
  off: "Not set up",
  idle: "Synced",
  syncing: "Syncing",
  failed: "Sync failed",
} as const;

function S3StatusCard() {
  const t = useT();
  const language = useLanguage();
  const status = useS3Status();
  const sync = useSyncS3();
  return (
    <section className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Sync status")}</h2>
        {status.data && (
          <span
            className={`xc-badge ${
              status.data.state === "failed"
                ? "danger"
                : status.data.state === "idle"
                  ? "ok"
                  : status.data.state === "syncing"
                    ? "info"
                    : ""
            }`}
          >
            {t(STATE_LABELS[status.data.state])}
          </span>
        )}
      </div>
      {status.isPending ? (
        <Loading />
      ) : status.isError ? (
        <ErrorState error={status.error} onRetry={() => status.refetch()} />
      ) : (
        <>
          <dl className="drive-status">
            <div>
              <dt>{t("Synced")}</dt>
              <dd>{status.data.synced}</dd>
            </div>
            <div>
              <dt>{t("Waiting to sync")}</dt>
              <dd>{status.data.pending}</dd>
            </div>
            <div>
              <dt>{t("Sync failed")}</dt>
              <dd>{status.data.failed}</dd>
            </div>
            <div>
              <dt>{t("Last sync")}</dt>
              <dd>
                {status.data.lastRunAt
                  ? relativeTime(status.data.lastRunAt, language)
                  : "—"}
              </dd>
            </div>
          </dl>
          {status.data.lastError && (
            <p className="xc-error-text">{status.data.lastError}</p>
          )}
          <p className="drive-muted drive-card-note">
            上传、改名、移动后会自动同步，每 10 分钟再全量对一遍。
          </p>
          <div className="drive-form-actions">
            <button
              type="button"
              className="xc-btn"
              disabled={status.data.state === "off" || sync.isPending}
              onClick={() => sync.mutate()}
            >
              <RefreshCw size={14} /> {t("Sync now")}
            </button>
          </div>
        </>
      )}
    </section>
  );
}

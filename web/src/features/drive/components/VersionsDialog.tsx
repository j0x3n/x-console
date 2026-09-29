import { lazy, Suspense, useEffect, useState } from "react";
import { Download, History, RotateCcw } from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  formatBytes,
  formatDate,
  formatTime,
  relativeTime,
} from "../../../lib/time";
import {
  isNotLive,
  readContent,
  readVersion,
  useDriveVersions,
  useRestoreVersion,
  useSaveVersionSettings,
  useVersionSettings,
  type DriveItem,
  type DriveVersion,
} from "../api";
import { canEdit, decodeUtf8 } from "../viewer/kind";

const DiffEditor = lazy(() =>
  import("../viewer/CodeEditor").then((m) => ({ default: m.DiffEditor })),
);

function asText(bytes: Uint8Array) {
  return decodeUtf8(bytes) ?? new TextDecoder("gbk").decode(bytes);
}

/**
 * 历史版本（B31）：左边是版本列表，右边是这个版本和当前内容的对比。
 * 文本文件按行对比（删掉的标红，新加的标绿）；其他文件只能下载或恢复。
 * 底部是保留规则，可以改。
 */
export default function VersionsDialog({
  item,
  onClose,
}: {
  item: DriveItem;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const versions = useDriveVersions(item.id);
  const restore = useRestoreVersion(item.id);
  const [picked, setPicked] = useState<number | null>(null);
  const list = versions.data ?? [];
  const selected = list.find((v) => v.id === picked) ?? list[0];

  if (versions.isError && isNotLive(versions.error))
    return (
      <Dialog
        open
        onClose={onClose}
        title={t("Version history")}
        description={item.name}
      >
        <NotLive name={t("Version history")} icon={<History size={28} />} />
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Close")}
          </button>
        </div>
      </Dialog>
    );

  const doRestore = async (v: DriveVersion) => {
    if (
      !(await confirmAction({
        title: `${t("Restore this version")}？`,
        description: `换成 ${formatDate(v.createdAt, language)} ${formatTime(v.createdAt, language)} 的内容。当前内容会先存成一个新版本，不会丢。`,
        confirmLabel: t("Restore"),
        danger: false,
      }))
    )
      return;
    restore.mutate(v.id, {
      onSuccess: () => {
        toast(t("Version restored"));
        onClose();
      },
    });
  };

  return (
    <Dialog
      open
      wide
      onClose={onClose}
      title={t("Version history")}
      description={item.name}
    >
      {versions.isPending ? (
        <Loading />
      ) : versions.isError ? (
        <ErrorState error={versions.error} onRetry={() => versions.refetch()} />
      ) : list.length === 0 ? (
        <EmptyState title={t("No older versions")} icon={<History size={26} />}>
          <span className="drive-muted">
            在线编辑并保存后，旧内容会存在这里。
          </span>
        </EmptyState>
      ) : (
        <div className="drive-versions">
          <ul className="drive-version-list" aria-label={t("Versions")}>
            {list.map((v) => (
              <li key={v.id}>
                <button
                  type="button"
                  className={v.id === selected?.id ? "active" : ""}
                  aria-pressed={v.id === selected?.id}
                  onClick={() => setPicked(v.id)}
                >
                  <span>{relativeTime(v.createdAt, language)}</span>
                  <small className="drive-muted">
                    {new Date(v.createdAt).toLocaleString(
                      language === "zh" ? "zh-CN" : "en",
                      { hour12: false },
                    )}{" "}
                    · {formatBytes(v.size)}
                  </small>
                </button>
              </li>
            ))}
          </ul>
          {selected && (
            <div className="drive-version-view">
              <div className="drive-version-bar">
                <small className="drive-muted">
                  {canEdit(item)
                    ? t("Red is this version, green is the current content")
                    : t("This file cannot be compared as text")}
                </small>
                <span className="xc-spacer" />
                <a
                  className="xc-btn small"
                  href={`/api/v1/drive/items/${item.id}/versions/${selected.id}/content`}
                  download={item.name}
                >
                  <Download size={14} />
                  <span className="drive-btn-text">{t("Download")}</span>
                </a>
                <button
                  type="button"
                  className="xc-btn small primary"
                  disabled={restore.isPending}
                  onClick={() => void doRestore(selected)}
                >
                  <RotateCcw size={14} /> {t("Restore this version")}
                </button>
              </div>
              {canEdit(item) && (
                <VersionDiff key={selected.id} item={item} version={selected} />
              )}
            </div>
          )}
        </div>
      )}
      <RetentionLine />
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn ghost" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

function VersionDiff({
  item,
  version,
}: {
  item: DriveItem;
  version: DriveVersion;
}) {
  const [texts, setTexts] = useState<{ old: string; cur: string } | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let alive = true;
    Promise.all([readVersion(item.id, version.id), readContent(item.id)])
      .then(([old, cur]) => {
        if (alive) setTexts({ old: asText(old), cur: asText(cur.bytes) });
      })
      .catch((e) => alive && setError(e));
    return () => {
      alive = false;
    };
  }, [item.id, version.id]);
  if (error) return <ErrorState error={error} />;
  if (!texts) return <Loading />;
  return (
    <div className="drive-version-diff">
      <Suspense fallback={<Loading />}>
        <DiffEditor name={item.name} original={texts.old} text={texts.cur} />
      </Suspense>
    </div>
  );
}

/** 保留规则：每个文件最多几个版本、最多几天。可以就地改。 */
function RetentionLine() {
  const t = useT();
  const settings = useVersionSettings();
  const save = useSaveVersionSettings();
  const [editing, setEditing] = useState(false);
  const [count, setCount] = useState(50);
  const [days, setDays] = useState(30);
  if (!settings.data) return null;
  if (!editing)
    return (
      <p className="drive-muted drive-retention">
        {t("Keeps the latest")} {settings.data.keepCount}{" "}
        {t("versions for up to")} {settings.data.keepDays} {t("days")}。
        <button
          type="button"
          className="drive-link"
          onClick={() => {
            setCount(settings.data.keepCount);
            setDays(settings.data.keepDays);
            setEditing(true);
          }}
        >
          {t("Change")}
        </button>
      </p>
    );
  return (
    <form
      className="drive-retention drive-retention-form"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate(
          { keepCount: count, keepDays: days },
          { onSuccess: () => setEditing(false) },
        );
      }}
    >
      <label>
        {t("Keep up to")}
        <input
          className="xc-input"
          type="number"
          min={1}
          max={500}
          value={count}
          onChange={(e) => setCount(Number(e.target.value) || 1)}
        />
        {t("versions")}
      </label>
      <label>
        {t("at most")}
        <input
          className="xc-input"
          type="number"
          min={1}
          max={3650}
          value={days}
          onChange={(e) => setDays(Number(e.target.value) || 1)}
        />
        {t("days")}
      </label>
      <button type="submit" className="xc-btn small" disabled={save.isPending}>
        {t("Save")}
      </button>
      <button
        type="button"
        className="xc-btn ghost small"
        onClick={() => setEditing(false)}
      >
        {t("Cancel")}
      </button>
    </form>
  );
}

import { lazy, Suspense, useEffect, useState } from "react";
import { AlertTriangle } from "lucide-react";
import { ApiError, errorMessage } from "../../../api/client";
import Markdown from "../../../components/markdown/Markdown";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { ErrorState, Loading } from "../../../components/ui/States";
import { Segmented } from "../../../components/ui/Toolbar";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { readContent, saveContent, type DriveItem } from "../api";
import { decodeUtf8 } from "./kind";

const CodeEditor = lazy(() => import("./CodeEditor"));
const DiffEditor = lazy(() =>
  import("./CodeEditor").then((m) => ({ default: m.DiffEditor })),
);

interface Loaded {
  text: string;
  etag: string;
  /** 不是 UTF-8，按 GBK 显示的。 */
  legacy: boolean;
  binary: boolean;
}

async function load(id: number): Promise<Loaded> {
  const { bytes, etag } = await readContent(id);
  if (bytes.subarray(0, 8192).includes(0))
    return { text: "", etag, legacy: false, binary: true };
  const utf8 = decodeUtf8(bytes);
  if (utf8 != null) return { text: utf8, etag, legacy: false, binary: false };
  return {
    text: new TextDecoder("gbk").decode(bytes),
    etag,
    legacy: true,
    binary: false,
  };
}

/**
 * 文本和 Markdown。默认只读，“编辑”后可以改和保存。
 * 保存时带上打开时的版本号，别处改过会提示对比或覆盖。
 */
export default function TextView({
  item,
  markdown,
  editing,
  onEditingChange,
  onDirtyChange,
  onSaved,
}: {
  item: DriveItem;
  markdown: boolean;
  editing: boolean;
  onEditingChange: (editing: boolean) => void;
  onDirtyChange: (dirty: boolean) => void;
  onSaved: (item: DriveItem) => void;
}) {
  const t = useT();
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [diff, setDiff] = useState<string | null>(null);
  const [source, setSource] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let alive = true;
    setLoaded(null);
    setError(null);
    load(item.id)
      .then((r) => {
        if (!alive) return;
        setLoaded(r);
        setDraft(r.text);
      })
      .catch((e) => alive && setError(e));
    return () => {
      alive = false;
    };
  }, [item.id, attempt]);

  const dirty = loaded != null && draft !== loaded.text;
  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty]);
  useEffect(() => () => onDirtyChange(false), []);

  const save = async (force = false) => {
    if (!loaded || saving || (!dirty && !force)) return;
    setSaving(true);
    try {
      const text = draft;
      const r = await saveContent(item.id, text, force ? "" : loaded.etag);
      setLoaded({ text, etag: r.etag, legacy: false, binary: false });
      setConflict(false);
      setDiff(null);
      toast(t("Saved"));
      onSaved(r.item);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) setConflict(true);
      else toast({ message: errorMessage(e), tone: "error" });
    } finally {
      setSaving(false);
    }
  };

  // 焦点不在编辑器里时也能 ⌘S / Ctrl+S。
  useEffect(() => {
    if (!editing) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        void save();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  });

  const stopEditing = async () => {
    if (
      dirty &&
      !(await confirmAction({
        title: t("Discard unsaved changes?"),
        description: "没保存的改动会丢掉。",
      }))
    )
      return;
    if (loaded) setDraft(loaded.text);
    setConflict(false);
    setDiff(null);
    onEditingChange(false);
  };

  const showDiff = async () => {
    try {
      const latest = await load(item.id);
      setDiff(latest.text);
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    }
  };

  const reload = async () => {
    if (
      dirty &&
      !(await confirmAction({
        title: t("Discard unsaved changes?"),
        description: "会换成服务器上的最新内容。",
      }))
    )
      return;
    setConflict(false);
    setDiff(null);
    setAttempt((n) => n + 1);
  };

  if (error)
    return (
      <div className="drive-viewer-center">
        <ErrorState error={error} onRetry={() => setAttempt((n) => n + 1)} />
      </div>
    );
  if (!loaded) return <Loading />;
  if (loaded.binary)
    return (
      <div className="drive-viewer-center">
        <p className="drive-muted">这个文件看起来不是文本，只能下载。</p>
      </div>
    );

  const rendered = markdown && !editing && !source;

  return (
    <div className="drive-text">
      {(editing || markdown || loaded.legacy || conflict) && (
        <div className="drive-text-bar">
          {conflict ? (
            <span className="drive-text-warn">
              <AlertTriangle size={14} /> {t("Changed elsewhere")}
            </span>
          ) : loaded.legacy ? (
            <span className="drive-text-warn">
              <AlertTriangle size={14} />
              不是 UTF-8，编辑保存后会改成 UTF-8。
            </span>
          ) : editing ? (
            <span className="drive-muted">
              {dirty ? t("Unsaved changes") : t("No changes")}
            </span>
          ) : null}
          <span className="xc-spacer" />
          {markdown && !editing && (
            <Segmented
              label={t("View")}
              value={source ? "source" : "rendered"}
              onChange={(v) => setSource(v === "source")}
              options={[
                { value: "rendered", label: t("Preview") },
                { value: "source", label: t("Source code") },
              ]}
            />
          )}
          {conflict && (
            <>
              <button
                type="button"
                className="xc-btn small"
                onClick={() => (diff == null ? showDiff() : setDiff(null))}
              >
                {diff == null ? t("Show changes") : t("Hide changes")}
              </button>
              <button type="button" className="xc-btn small" onClick={reload}>
                {t("Reload")}
              </button>
              <button
                type="button"
                className="xc-btn small danger"
                disabled={saving}
                onClick={() => save(true)}
              >
                {t("Overwrite")}
              </button>
            </>
          )}
          {editing && !conflict && (
            <>
              <button
                type="button"
                className="xc-btn small"
                onClick={stopEditing}
              >
                {t("Stop editing")}
              </button>
              <button
                type="button"
                className="xc-btn small primary"
                disabled={!dirty || saving}
                onClick={() => save()}
                title="⌘S / Ctrl+S"
              >
                {saving ? t("Saving") : t("Save")}
              </button>
            </>
          )}
        </div>
      )}
      <div className="drive-text-body">
        {rendered ? (
          <div className="drive-markdown">
            <Markdown source={loaded.text} />
          </div>
        ) : (
          <Suspense fallback={<Loading />}>
            {diff != null ? (
              <DiffEditor
                key={`${diff.length}:${draft.length}`}
                name={item.name}
                original={diff}
                text={draft}
              />
            ) : (
              <CodeEditor
                name={item.name}
                text={draft}
                editable={editing}
                onChange={setDraft}
                onSave={() => void save()}
              />
            )}
          </Suspense>
        )}
      </div>
    </div>
  );
}

import { useEffect, useState } from "react";
import {
  CheckCheck,
  Download,
  ExternalLink,
  Pencil,
  RefreshCw,
  Sparkles,
  Trash2,
  Undo2,
} from "lucide-react";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import Dialog from "../../components/ui/Dialog";
import { Spinner } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import { errorMessage } from "../../api/client";
import {
  useDeleteReadItem,
  useReadItem,
  useRefetchReadItem,
  useSummarizeReadItem,
  useUpdateReadItem,
} from "./api";
import ReadingBody from "./ReadingBody";
import { SOURCE_LABELS, parseTags } from "./format";
import "./i18n";

function showError(err: unknown) {
  toast({ message: errorMessage(err), tone: "error" });
}

/** 一条链接的详情：摘要、标签、备注、存下来的正文，以及已读、重抓、删除。 */
export default function ReadDetail({
  id,
  onClose,
}: {
  id: number | null;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const query = useReadItem(id);
  const update = useUpdateReadItem();
  const remove = useDeleteReadItem();
  const refetch = useRefetchReadItem();
  const summarize = useSummarizeReadItem();
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState("");
  const [tags, setTags] = useState("");
  const [note, setNote] = useState("");
  const item = query.data;

  useEffect(() => {
    setEditing(false);
  }, [id]);
  useEffect(() => {
    if (!editing || !item) return;
    setTitle(item.title);
    setTags(item.tags.join(", "));
    setNote(item.note);
    // 只在进入编辑时取一次值，抓取完成后不覆盖正在输入的内容
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editing]);

  if (id == null) return null;

  const save = () => {
    if (!item) return;
    update.mutate(
      { id: item.id, body: { title, tags: parseTags(tags), note } },
      {
        onSuccess: () => {
          toast(t("Saved"));
          setEditing(false);
        },
        onError: showError,
      },
    );
  };
  const toggleRead = () => {
    if (!item) return;
    update.mutate(
      { id: item.id, body: { read: !item.read } },
      { onSuccess: item.read ? undefined : onClose, onError: showError },
    );
  };
  const onDelete = async () => {
    if (!item) return;
    const ok = await confirmAction({
      title: `${t("Delete this saved link?")} ${item.title}`,
      description: t("The saved page text and pictures are deleted too."),
      confirmLabel: t("Delete"),
    });
    if (!ok) return;
    remove.mutate(item.id, {
      onSuccess: () => {
        toast(t("Deleted"));
        onClose();
      },
      onError: showError,
    });
  };

  return (
    <Dialog open onClose={onClose} title={item?.title ?? t("Read later")} wide>
      {query.isPending && <Spinner />}
      {query.isError && (
        <p className="xc-error-text">{errorMessage(query.error)}</p>
      )}
      {item && !editing && (
        <>
          <p className="readlater-meta">
            <a href={item.url} target="_blank" rel="noopener noreferrer">
              <ExternalLink size={13} /> {item.site || item.url}
            </a>
            <span className="xc-muted">
              {relativeTime(item.createdAt, language)} ·{" "}
              {t(SOURCE_LABELS[item.source])}
            </span>
            {item.status === "queued" && (
              <span className="xc-badge info">{t("Fetching…")}</span>
            )}
            {item.status === "failed" && (
              <span className="xc-badge danger">{t("Fetch failed")}</span>
            )}
          </p>
          {item.status === "failed" && item.error && (
            <p className="xc-error-text">{item.error}</p>
          )}
          {item.summary && <p className="readlater-summary">{item.summary}</p>}
          {item.tags.length > 0 && (
            <p className="readlater-tagline">
              {item.tags.map((tag) => (
                <span key={tag} className="xc-badge">
                  {tag}
                </span>
              ))}
            </p>
          )}
          {item.note && <p className="readlater-note">{item.note}</p>}
          {item.meta.incomplete === true && (
            <p className="readlater-warn">
              {t(
                "This post may be cut short. Add your X login cookie in Settings → Read later, then fetch again.",
              )}
            </p>
          )}
          {item.status === "ready" && !item.hasHtml && item.hasContent && (
            <p className="xc-muted">
              {t(
                "This item only has text. Fetch it again to keep the pictures and layout.",
              )}
            </p>
          )}
          {item.contentHtml && <ReadingBody html={item.contentHtml} />}
          <details className="readlater-text">
            <summary>{t("Saved page text")}</summary>
            {item.content ? (
              <pre>{item.content}</pre>
            ) : (
              <p className="xc-muted">{t("No page text was saved.")}</p>
            )}
          </details>
        </>
      )}
      {item && editing && (
        <>
          <label className="xc-field">
            <span>{t("Title")}</span>
            <input
              className="xc-input"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Tags, separated by commas")}</span>
            <input
              className="xc-input"
              value={tags}
              onChange={(e) => setTags(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Note on this link")}</span>
            <textarea
              className="xc-input"
              rows={3}
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </label>
        </>
      )}
      {item && (
        <div className="xc-dialog-actions readlater-actions">
          {editing ? (
            <>
              <span className="xc-spacer" />
              <button className="xc-btn" onClick={() => setEditing(false)}>
                {t("Cancel")}
              </button>
              <button
                className="xc-btn primary"
                disabled={update.isPending || !title.trim()}
                onClick={save}
              >
                {t("Save")}
              </button>
            </>
          ) : (
            <>
              <button
                className="xc-btn ghost danger"
                disabled={remove.isPending}
                onClick={() => void onDelete()}
              >
                <Trash2 size={14} /> {t("Delete")}
              </button>
              <button
                className="xc-btn ghost"
                disabled={refetch.isPending || item.status === "queued"}
                onClick={() =>
                  refetch.mutate(item.id, {
                    onSuccess: () => toast(t("Fetching again…")),
                    onError: showError,
                  })
                }
              >
                <RefreshCw size={14} /> {t("Fetch again")}
              </button>
              {item.hasHtml && (
                <a
                  className="xc-btn ghost"
                  href={`/api/v1/readlater/${item.id}/export`}
                  download
                  title={t("Download one HTML file with the pictures inside")}
                >
                  <Download size={14} /> {t("Download")}
                </a>
              )}
              <button
                className="xc-btn ghost"
                disabled={summarize.isPending || !item.hasContent}
                onClick={() =>
                  summarize.mutate(item.id, {
                    onSuccess: () => toast(t("Summary updated")),
                    onError: showError,
                  })
                }
              >
                <Sparkles size={14} />{" "}
                {summarize.isPending
                  ? t("Writing summary…")
                  : t("Write summary")}
              </button>
              <span className="xc-spacer" />
              <button className="xc-btn" onClick={() => setEditing(true)}>
                <Pencil size={14} /> {t("Edit")}
              </button>
              <button
                className="xc-btn primary"
                disabled={update.isPending}
                onClick={toggleRead}
              >
                {item.read ? <Undo2 size={14} /> : <CheckCheck size={14} />}{" "}
                {item.read ? t("Mark as unread") : t("Mark as read")}
              </button>
            </>
          )}
        </div>
      )}
    </Dialog>
  );
}

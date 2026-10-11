import { useEffect, useState } from "react";
import { Check, RefreshCw, Trash2, X } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useProjects } from "../projects/api";
import {
  useDeleteInbox,
  useDismissInbox,
  useFileInbox,
  useSuggestInbox,
  type InboxItem,
  type InboxKind,
  type InboxSettings,
} from "./api";
import {
  KIND_OPTIONS,
  KIND_ZH,
  SOURCE_LABELS,
  formFrom,
  fromLocalInput,
} from "./format";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 一条没处理的条目：原文、AI 建议（可改）、归档和忽略。 */
export default function ItemCard({
  item,
  settings,
}: {
  item: InboxItem;
  settings?: InboxSettings;
}) {
  const t = useT();
  const projects = useProjects();
  const file = useFileInbox();
  const dismiss = useDismissInbox();
  const remove = useDeleteInbox();
  const suggest = useSuggestInbox();
  const [form, setForm] = useState(() => formFrom(item));
  const [projectId, setProjectId] = useState<number>(
    settings?.cardProjectId ?? 0,
  );
  const [error, setError] = useState("");
  const key = `${item.id}:${item.suggestState}:${item.suggestion?.title ?? ""}:${item.suggestion?.kind ?? ""}`;

  // 建议出来或重新生成后，用新的建议刷新表单
  useEffect(() => {
    setForm(formFrom(item));
    setError("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  useEffect(() => {
    if (projectId === 0 && projects.data?.length)
      setProjectId(settings?.cardProjectId || projects.data[0].id);
  }, [projects.data, settings?.cardProjectId, projectId]);

  const set = (patch: Partial<typeof form>) =>
    setForm((f) => ({ ...f, ...patch }));
  const kind = form.kind;

  const submit = () => {
    setError("");
    if (kind === "reminder" && !form.at)
      return setError(t("A reminder needs a time"));
    if (kind === "readlater" && !form.url.trim())
      return setError(t("Read later needs an address"));
    if (kind === "card" && !projectId) return setError(t("Pick a project"));
    file.mutate(
      {
        id: item.id,
        body: {
          kind,
          title: form.title.trim(),
          body: form.body,
          at: kind === "reminder" ? fromLocalInput(form.at) : "",
          url: form.url.trim(),
          ...(kind === "card" ? { projectId } : {}),
        },
      },
      {
        onSuccess: (done) =>
          toast(`${t("Filed as")}${KIND_ZH[done.filedKind ?? kind]}`),
        onError: (err) => setError(errorMessage(err)),
      },
    );
  };

  return (
    <article className="xc-card inbox-item" aria-label={item.text.slice(0, 30)}>
      <div className="inbox-item-head">
        <p className="inbox-item-text">{item.text}</p>
        <span className="xc-muted inbox-item-meta">
          {t(SOURCE_LABELS[item.source])} ·{" "}
          {new Date(item.createdAt).toLocaleString()}
        </span>
      </div>
      <div className="inbox-suggest" aria-live="polite">
        {item.suggestState === "pending" && (
          <span className="xc-muted">
            {t("Thinking about where this belongs…")}
          </span>
        )}
        {item.suggestState === "failed" && (
          <span className="xc-muted">
            {t("The suggestion failed. Pick a place yourself or try again.")}
          </span>
        )}
        {item.suggestState === "off" && (
          <span className="xc-muted">
            {t("No suggestion. Pick a place yourself.")}
          </span>
        )}
        {item.suggestState === "ready" && item.suggestion?.reason && (
          <span className="xc-muted">{`${t("Suggestion")}：${item.suggestion.reason}`}</span>
        )}
      </div>
      <div className="inbox-form">
        <label className="xc-field">
          <span>{t("File as")}</span>
          <select
            className="xc-select"
            value={kind}
            onChange={(e) => set({ kind: e.target.value as InboxKind })}
          >
            {KIND_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {t(o.label)}
              </option>
            ))}
          </select>
        </label>
        {kind !== "readlater" && (
          <label className="xc-field">
            <span>{t("Inbox title")}</span>
            <input
              className="xc-input"
              value={form.title}
              maxLength={200}
              onChange={(e) => set({ title: e.target.value })}
            />
          </label>
        )}
        {kind === "reminder" && (
          <label className="xc-field">
            <span>{t("Inbox remind at")}</span>
            <input
              className="xc-input"
              type="datetime-local"
              value={form.at}
              onChange={(e) => set({ at: e.target.value })}
            />
          </label>
        )}
        {kind === "card" && (
          <label className="xc-field">
            <span>{t("Project")}</span>
            <select
              className="xc-select"
              value={projectId}
              onChange={(e) => setProjectId(Number(e.target.value))}
            >
              {projects.data?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
        )}
        {kind === "readlater" && (
          <label className="xc-field">
            <span>{t("Inbox address")}</span>
            <input
              className="xc-input"
              value={form.url}
              onChange={(e) => set({ url: e.target.value })}
              placeholder="https://"
            />
          </label>
        )}
        {(kind === "note" || kind === "card") && (
          <label className="xc-field inbox-wide">
            <span>{t("Inbox details")}</span>
            <textarea
              className="xc-textarea"
              rows={2}
              value={form.body}
              onChange={(e) => set({ body: e.target.value })}
            />
          </label>
        )}
      </div>
      {error && (
        <p className="xc-error" role="alert">
          {error}
        </p>
      )}
      <div className="inbox-actions">
        <button
          className="xc-btn small primary"
          disabled={file.isPending}
          onClick={submit}
        >
          <Check size={14} /> {t("File it")}
        </button>
        <button
          className="xc-btn small"
          disabled={dismiss.isPending}
          onClick={() => dismiss.mutate(item.id, { onError: fail })}
        >
          <X size={14} /> {t("Inbox dismiss")}
        </button>
        <button
          className="xc-btn small"
          disabled={suggest.isPending || item.suggestState === "pending"}
          onClick={() => suggest.mutate(item.id, { onError: fail })}
        >
          <RefreshCw size={14} /> {t("Suggest again")}
        </button>
        <button
          className="icon-button"
          title={t("Delete")}
          aria-label={`${t("Delete")} ${item.text.slice(0, 20)}`}
          onClick={() => remove.mutate(item.id, { onError: fail })}
        >
          <Trash2 size={14} />
        </button>
      </div>
    </article>
  );
}

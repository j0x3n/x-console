import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  AlarmClock,
  Archive,
  ArchiveRestore,
  ArrowLeft,
  Ellipsis,
  Eye,
  Pencil,
  Pin,
  PinOff,
  SquareKanban,
  Trash2,
  X,
} from "lucide-react";
import { ErrorState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import Markdown from "../../projects/Markdown";
import {
  notesKeys,
  patchNote,
  patchNoteKeepalive,
  useDeleteNote,
  useNote,
  useUpdateNote,
  type Note,
} from "../api";
import { AutoSaver, type SaveState } from "../autosave";
import { parseTags, sameTags } from "../logic";
import ToIssueDialog from "./ToIssueDialog";
import ToReminderDialog from "./ToReminderDialog";

interface Draft {
  title: string;
  body: string;
  tags: string[];
}

const sameDraft = (a: Draft, b: Draft) =>
  a.title === b.title && a.body === b.body && sameTags(a.tags, b.tags);

export default function NoteEditor({
  id,
  backTo,
}: {
  id: number;
  backTo: string;
}) {
  const note = useNote(id);
  if (note.isPending) return <Loading />;
  if (note.isError)
    return <ErrorState error={note.error} onRetry={() => note.refetch()} />;
  return <EditorBody key={id} note={note.data} backTo={backTo} />;
}

function EditorBody({ note, backTo }: { note: Note; backTo: string }) {
  const t = useT();
  const language = useLanguage();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const update = useUpdateNote();
  const remove = useDeleteNote();
  const [draft, setDraft] = useState<Draft>({
    title: note.title,
    body: note.body,
    tags: note.tags,
  });
  const [tagInput, setTagInput] = useState("");
  const [preview, setPreview] = useState(false);
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [menuOpen, setMenuOpen] = useState(false);
  const [dialog, setDialog] = useState<"issue" | "reminder" | null>(null);
  const lastSaved = useRef<Draft>({
    title: note.title,
    body: note.body,
    tags: note.tags,
  });
  const saver = useRef<AutoSaver<Draft>>(null);
  if (!saver.current)
    saver.current = new AutoSaver<Draft>({
      delay: 800,
      onState: setSaveState,
      save: async (d) => {
        const saved = await patchNote(note.id, d);
        lastSaved.current = d;
        qc.setQueryData(notesKeys.note(note.id), saved);
        qc.invalidateQueries({ queryKey: notesKeys.lists });
        qc.invalidateQueries({ queryKey: notesKeys.tags });
      },
    });

  // 另一个窗口改了这条笔记：本地没有未保存的改动时，采用新内容。
  useEffect(() => {
    const server: Draft = {
      title: note.title,
      body: note.body,
      tags: note.tags,
    };
    if (saver.current?.dirty || sameDraft(server, lastSaved.current)) return;
    lastSaved.current = server;
    setDraft(server);
  }, [note.title, note.body, note.tags]);

  // 离开时把没保存的内容存掉。页面关闭用 keepalive 请求。
  useEffect(() => {
    const s = saver.current!;
    const onHide = () => {
      if (s.unsaved) patchNoteKeepalive(note.id, s.unsaved);
    };
    const onVisibility = () => {
      if (document.visibilityState === "hidden") void s.flush();
    };
    window.addEventListener("pagehide", onHide);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("pagehide", onHide);
      document.removeEventListener("visibilitychange", onVisibility);
      void s.flush();
      s.dispose();
    };
  }, [note.id]);

  const edit = (next: Partial<Draft>) => {
    const merged = { ...draft, ...next };
    setDraft(merged);
    saver.current!.change(merged);
  };
  const addTags = (input: string) => {
    const tags = parseTags(input);
    if (tags.length)
      edit({
        tags: [...draft.tags, ...tags.filter((x) => !draft.tags.includes(x))],
      });
    setTagInput("");
  };
  // 服务端改了正文（比如转 Issue 后加了链接），直接采用。
  const adopt = (n: Note) => {
    const server = { title: n.title, body: n.body, tags: n.tags };
    lastSaved.current = server;
    setDraft(server);
  };
  const flushThen = async (fn: () => void) => {
    await saver.current!.flush();
    fn();
  };

  const status =
    saveState === "pending" || saveState === "saving"
      ? t("Saving…")
      : saveState === "error"
        ? t("Save failed. Retrying soon.")
        : saveState === "saved"
          ? t("Saved")
          : `${t("Edited")} ${relativeTime(note.updatedAt, language)}`;

  return (
    <div className="notes-editor">
      <header className="notes-editor-head">
        <Link
          to={backTo}
          className="xc-btn ghost small notes-back"
          aria-label={t("Back to list")}
        >
          <ArrowLeft size={15} />
        </Link>
        <span className={`notes-save-state ${saveState}`} role="status">
          {status}
        </span>
        <span className="xc-spacer" />
        {note.archivedAt && (
          <span className="xc-badge warn">{t("Archived")}</span>
        )}
        <button
          className="xc-btn ghost small"
          onClick={() => setPreview((v) => !v)}
          aria-pressed={preview}
          title={preview ? t("Edit") : t("Preview")}
        >
          {preview ? <Pencil size={14} /> : <Eye size={14} />}
          <span className="notes-btn-text">
            {preview ? t("Edit") : t("Preview")}
          </span>
        </button>
        <button
          className={`xc-btn ghost small ${note.pinned ? "notes-pinned" : ""}`}
          onClick={() =>
            update.mutate({ id: note.id, body: { pinned: !note.pinned } })
          }
          aria-pressed={note.pinned}
          title={note.pinned ? t("Unpin") : t("Pin")}
        >
          {note.pinned ? <PinOff size={14} /> : <Pin size={14} />}
        </button>
        <div className="notes-menu-wrap">
          <button
            className="xc-btn ghost small"
            aria-label={t("More")}
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen((v) => !v)}
          >
            <Ellipsis size={15} />
          </button>
          {menuOpen && (
            <div
              className="notes-menu"
              role="menu"
              onClick={() => setMenuOpen(false)}
            >
              <button
                role="menuitem"
                onClick={() => flushThen(() => setDialog("issue"))}
              >
                <SquareKanban size={14} /> {t("Turn into issue")}
              </button>
              <button
                role="menuitem"
                onClick={() => flushThen(() => setDialog("reminder"))}
              >
                <AlarmClock size={14} /> {t("Remind me")}
              </button>
              <button
                role="menuitem"
                onClick={() =>
                  update.mutate({
                    id: note.id,
                    body: { archived: !note.archivedAt },
                  })
                }
              >
                {note.archivedAt ? (
                  <ArchiveRestore size={14} />
                ) : (
                  <Archive size={14} />
                )}
                {note.archivedAt ? t("Unarchive") : t("Archive")}
              </button>
              <button
                role="menuitem"
                className="danger"
                onClick={() => {
                  if (!confirm(t("Delete this note? This cannot be undone.")))
                    return;
                  saver.current!.dispose();
                  remove.mutate(note.id, { onSuccess: () => navigate(backTo) });
                }}
              >
                <Trash2 size={14} /> {t("Delete")}
              </button>
            </div>
          )}
        </div>
      </header>
      <input
        className="notes-title-input"
        value={draft.title}
        placeholder={t("Title")}
        aria-label={t("Title")}
        onChange={(e) => edit({ title: e.target.value })}
      />
      <div className="notes-tags-edit">
        {draft.tags.map((tag) => (
          <span key={tag} className="notes-tag">
            #{tag}
            <button
              aria-label={`${t("Remove tag")} ${tag}`}
              onClick={() =>
                edit({ tags: draft.tags.filter((x) => x !== tag) })
              }
            >
              <X size={11} />
            </button>
          </span>
        ))}
        <input
          value={tagInput}
          placeholder={draft.tags.length ? "" : t("Add tags")}
          aria-label={t("Add tags")}
          onChange={(e) => setTagInput(e.target.value)}
          onKeyDown={(e) => {
            if ((e.key === "Enter" || e.key === ",") && tagInput.trim()) {
              e.preventDefault();
              addTags(tagInput);
            } else if (
              e.key === "Backspace" &&
              !tagInput &&
              draft.tags.length
            ) {
              edit({ tags: draft.tags.slice(0, -1) });
            }
          }}
          onBlur={() => tagInput.trim() && addTags(tagInput)}
        />
      </div>
      {preview ? (
        <div className="notes-preview">
          <Markdown
            source={draft.body}
            empty={<span className="xc-muted">{t("Nothing to preview")}</span>}
          />
        </div>
      ) : (
        <textarea
          className="notes-body-input"
          value={draft.body}
          placeholder={t("Write something. Markdown is supported.")}
          aria-label={t("Note")}
          autoFocus={!note.body}
          onChange={(e) => edit({ body: e.target.value })}
          onKeyDown={(e) => {
            if (e.key === "s" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              void saver.current!.flush();
            }
          }}
        />
      )}
      {dialog === "issue" && (
        <ToIssueDialog
          open
          onClose={() => setDialog(null)}
          noteId={note.id}
          onDone={adopt}
        />
      )}
      {dialog === "reminder" && (
        <ToReminderDialog
          open
          onClose={() => setDialog(null)}
          noteId={note.id}
        />
      )}
    </div>
  );
}

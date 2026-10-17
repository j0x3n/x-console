import { useEffect, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Link2Off, Pencil } from "lucide-react";
import { ApiError, errorMessage, unwrap } from "../../../api/client";
import Markdown from "../../../components/markdown/Markdown";
import MarkdownEditor from "../../../components/markdown/MarkdownEditor";
import { toast } from "../../../hooks/useToast";
import { EmptyState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { relativeTime } from "../../../lib/time";
import { notesApi } from "../api";
import { noteBgClass } from "../noteColors";
import "../i18n";
import "../notes.css";

/*
 * 笔记外链（B72）：https://<面板>/n/<token>，不用登录。
 * 只显示笔记的标题、正文和图片，没有任何按钮。
 * 有密码时先输密码，服务端发一个 12 小时的访问令牌，存在 sessionStorage。
 * B243：链接允许编辑时有“编辑”按钮，访客能改标题和正文，点“保存”才保存。
 */

const KEY = (token: string) => `xc.note-share.${token}`;

function readAccess(token: string) {
  try {
    return sessionStorage.getItem(KEY(token)) ?? "";
  } catch {
    return "";
  }
}

const getPublicNote = (token: string, access: string) =>
  unwrap(
    notesApi.GET("/public/notes/{token}", {
      params: { path: { token }, query: { t: access || undefined } },
    }),
  );

type PublicNote = Awaited<ReturnType<typeof getPublicNote>>;

const NAME_KEY = "xc.note-share.name";

function readName() {
  try {
    return localStorage.getItem(NAME_KEY) ?? "";
  } catch {
    return "";
  }
}

function writeName(name: string) {
  try {
    localStorage.setItem(NAME_KEY, name);
  } catch {
    /* 存不了下次再填 */
  }
}

function writeAccess(token: string, access: string) {
  try {
    sessionStorage.setItem(KEY(token), access);
  } catch {
    /* 存不了就只在这次页面里有效 */
  }
}

export default function NoteSharePage({ token }: { token: string }) {
  const t = useT();
  const language = useLanguage();
  const [access, setAccess] = useState(() => readAccess(token));
  const [editing, setEditing] = useState(false);
  const queryKey = ["note-share", token, access];
  const note = useQuery({
    queryKey,
    queryFn: () => getPublicNote(token, access),
    retry: false,
    meta: { silentError: true },
  });

  useEffect(() => {
    document.title = note.data?.title
      ? `${note.data.title} · X Console`
      : "X Console";
  }, [note.data]);

  let body;
  if (note.isPending) body = <Loading />;
  else if (note.isError) {
    const e = note.error;
    if (e instanceof ApiError && e.code === "note_password_required")
      body = (
        <PasswordForm
          token={token}
          onUnlocked={(a) => {
            writeAccess(token, a);
            setAccess(a);
          }}
        />
      );
    else if (e instanceof ApiError && e.status === 404)
      body = (
        <EmptyState
          title={t("This link does not work")}
          icon={<Link2Off size={28} />}
        >
          <span className="xc-muted">
            链接可能已经过期、被取消，或者笔记已经删掉了。
          </span>
        </EmptyState>
      );
    else
      body = (
        <EmptyState
          title={t("Could not open this link")}
          icon={<Link2Off size={28} />}
        >
          <span className="xc-muted">
            {e instanceof ApiError && e.status === 501
              ? "服务端还没上线笔记分享。"
              : errorMessage(e)}
          </span>
        </EmptyState>
      );
  } else if (editing)
    body = (
      <ShareEditor
        token={token}
        access={access}
        note={note.data}
        queryKey={queryKey}
        onDone={() => setEditing(false)}
      />
    );
  else
    body = (
      <article className={`note-share-doc ${noteBgClass(note.data.color)}`}>
        {note.data.canEdit && (
          <div className="note-share-tools">
            <button
              type="button"
              className="xc-btn small"
              onClick={() => setEditing(true)}
            >
              <Pencil size={13} /> {t("Edit")}
            </button>
          </div>
        )}
        {note.data.title.trim() && <h1>{note.data.title}</h1>}
        <Markdown className="notes-preview" source={note.data.body} />
        <footer className="note-share-meta">
          {t("Updated")} {relativeTime(note.data.updatedAt, language)}
        </footer>
      </article>
    );

  return (
    <div className="note-share-page">
      <main className="note-share-main">{body}</main>
      <p className="note-share-brand">{t("Shared with X Console")}</p>
    </div>
  );
}

/** B243：访客编辑。保存时带上开始编辑时的 updatedAt，别人改过就提示。 */
function ShareEditor({
  token,
  access,
  note,
  queryKey,
  onDone,
}: {
  token: string;
  access: string;
  note: PublicNote;
  queryKey: unknown[];
  onDone: () => void;
}) {
  const t = useT();
  const qc = useQueryClient();
  const [title, setTitle] = useState(note.title);
  const [body, setBody] = useState(note.body);
  const [base, setBase] = useState(note.updatedAt);
  const [name, setName] = useState(readName);
  const [saving, setSaving] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [mine, setMine] = useState("");
  const dirty = title !== note.title || body !== note.body;

  // 有没保存的改动时，关页面前浏览器会提示。
  useEffect(() => {
    if (!dirty) return;
    const onLeave = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", onLeave);
    return () => window.removeEventListener("beforeunload", onLeave);
  }, [dirty]);

  const save = async () => {
    setSaving(true);
    const editorName = name.trim();
    writeName(editorName);
    try {
      const saved = await unwrap(
        notesApi.PUT("/public/notes/{token}", {
          params: { path: { token }, query: { t: access || undefined } },
          body: {
            title,
            body,
            baseUpdatedAt: base,
            editorName: editorName || undefined,
          },
        }),
      );
      qc.setQueryData(queryKey, saved);
      toast(t("Saved"));
      onDone();
    } catch (error) {
      if (error instanceof ApiError && error.code === "note_changed")
        setConflict(true);
      else {
        toast({ message: errorMessage(error), tone: "error" });
        if (error instanceof ApiError && error.status !== 400) {
          // 链接关了编辑、过期了或者要重新输密码：回到只读页面重新读。
          onDone();
          void qc.invalidateQueries({ queryKey });
        }
      }
    } finally {
      setSaving(false);
    }
  };

  const loadLatest = async () => {
    try {
      const latest = await getPublicNote(token, access);
      qc.setQueryData(queryKey, latest);
      setMine(body);
      setTitle(latest.title);
      setBody(latest.body);
      setBase(latest.updatedAt);
      setConflict(false);
    } catch (error) {
      toast({ message: errorMessage(error), tone: "error" });
    }
  };

  return (
    <article
      className={`note-share-doc note-share-editing ${noteBgClass(note.color)}`}
    >
      {conflict && (
        <div className="note-share-conflict" role="alert">
          <span>{t("This note was just changed")}</span>
          <span className="xc-spacer" />
          <button
            type="button"
            className="xc-btn small"
            onClick={() => setConflict(false)}
          >
            {t("Cancel")}
          </button>
          <button
            type="button"
            className="xc-btn small primary"
            onClick={() => void loadLatest()}
          >
            {t("Load the latest")}
          </button>
        </div>
      )}
      <input
        className="note-share-title-input"
        value={title}
        maxLength={200}
        placeholder={t("Note title")}
        aria-label={t("Note title")}
        onChange={(e) => setTitle(e.target.value)}
      />
      <div className="xc-field">
        <MarkdownEditor
          value={body}
          onChange={setBody}
          label={t("Note")}
          minRows={12}
          autoFocus
        />
      </div>
      {mine && (
        <label className="xc-field">
          <span>{t("What you wrote. Copy what you need.")}</span>
          <textarea className="xc-textarea" readOnly rows={6} value={mine} />
        </label>
      )}
      <div className="note-share-edit-foot">
        <input
          className="xc-input"
          value={name}
          maxLength={20}
          placeholder={t("Your name (optional)")}
          aria-label={t("Your name (optional)")}
          onChange={(e) => setName(e.target.value)}
        />
        <span className="xc-spacer" />
        <button type="button" className="xc-btn" onClick={onDone}>
          {t("Cancel")}
        </button>
        <button
          type="button"
          className="xc-btn primary"
          disabled={saving}
          onClick={() => void save()}
        >
          {t("Save")}
        </button>
      </div>
    </article>
  );
}

function PasswordForm({
  token,
  onUnlocked,
}: {
  token: string;
  onUnlocked: (access: string) => void;
}) {
  const t = useT();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!password) return;
    setBusy(true);
    setError("");
    try {
      const r = await unwrap(
        notesApi.POST("/public/notes/{token}/unlock", {
          params: { path: { token } },
          body: { password },
        }),
      );
      onUnlocked(r.access);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="note-share-lock" onSubmit={(e) => void submit(e)}>
      <KeyRound size={28} className="xc-muted" />
      <strong>{t("This note needs a password")}</strong>
      <input
        className="xc-input"
        type="password"
        autoFocus
        value={password}
        maxLength={32}
        aria-label={t("Password")}
        onChange={(e) => setPassword(e.target.value)}
      />
      {error && <small className="xc-error-text">{error}</small>}
      <button type="submit" className="xc-btn primary" disabled={busy}>
        {t("Open")}
      </button>
    </form>
  );
}

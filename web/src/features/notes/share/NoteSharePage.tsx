import { useEffect, useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { KeyRound, Link2Off } from "lucide-react";
import { ApiError, errorMessage, unwrap } from "../../../api/client";
import Markdown from "../../../components/markdown/Markdown";
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
 */

const KEY = (token: string) => `xc.note-share.${token}`;

function readAccess(token: string) {
  try {
    return sessionStorage.getItem(KEY(token)) ?? "";
  } catch {
    return "";
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
  const note = useQuery({
    queryKey: ["note-share", token, access],
    queryFn: () =>
      unwrap(
        notesApi.GET("/public/notes/{token}", {
          params: { path: { token }, query: { t: access || undefined } },
        }),
      ),
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
  } else
    body = (
      <article className={`note-share-doc ${noteBgClass(note.data.color)}`}>
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

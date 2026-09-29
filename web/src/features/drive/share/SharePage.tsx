import { useEffect, useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ChevronRight,
  Download,
  FolderOpen,
  KeyRound,
  Link2Off,
} from "lucide-react";
import { ApiError, errorMessage, unwrap } from "../../../api/client";
import { EmptyState, Loading } from "../../../components/ui/States";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatBytes, relativeTime } from "../../../lib/time";
import { driveApi } from "../api";
import FileIcon from "../components/FileIcon";
import { fileKind } from "../logic";
import "../i18n";
import "../drive.css";

/*
 * 分享页（B31）：https://<面板>/s/<token>，不用登录。
 * - 有提取码时先输提取码，服务端发一个 1 小时的访问令牌，存在 sessionStorage。
 * - 文件：显示信息，图片、视频、音频、PDF 可以直接看，下面是“下载”。
 * - 文件夹：可以点进子文件夹，单个文件下载，或者整个打包下载。
 * - 只能看到分享的这一个文件或文件夹里的东西。
 */

const KEY = (token: string) => `xc.share.${token}`;

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

/** 公开接口的地址，带上访问令牌。 */
export function publicUrl(
  token: string,
  part: "content" | "zip",
  opts: { access?: string; item?: number; inline?: boolean } = {},
) {
  const q = new URLSearchParams();
  if (opts.access) q.set("t", opts.access);
  if (opts.item) q.set("item", String(opts.item));
  if (opts.inline) q.set("inline", "1");
  const qs = q.toString();
  return `/api/v1/public/shares/${encodeURIComponent(token)}/${part}${qs ? `?${qs}` : ""}`;
}

export default function SharePage({ token }: { token: string }) {
  const t = useT();
  const [access, setAccess] = useState(() => readAccess(token));
  const info = useQuery({
    queryKey: ["share", token, access],
    queryFn: () =>
      unwrap(
        driveApi.GET("/public/shares/{token}", {
          params: { path: { token }, query: { t: access || undefined } },
        }),
      ),
    retry: false,
  });

  useEffect(() => {
    document.title = info.data ? `${info.data.name} · X Console` : "X Console";
  }, [info.data]);

  let body;
  if (info.isPending) body = <Loading />;
  else if (info.isError) {
    const e = info.error;
    if (e instanceof ApiError && e.code === "share_code_required")
      body = (
        <CodeForm
          token={token}
          onUnlocked={(a) => {
            writeAccess(token, a);
            setAccess(a);
          }}
        />
      );
    else if (
      e instanceof ApiError &&
      (e.status === 501 || e.code === "not_live")
    )
      body = (
        <EmptyState
          title={t("Sharing is not live yet")}
          icon={<Link2Off size={28} />}
        >
          <span className="drive-muted">服务端还没上线分享功能。</span>
        </EmptyState>
      );
    else if (e instanceof ApiError && e.status === 404)
      body = (
        <EmptyState
          title={t("This link does not work")}
          icon={<Link2Off size={28} />}
        >
          <span className="drive-muted">
            链接可能已经过期、被取消，或者文件已经删掉了。
          </span>
        </EmptyState>
      );
    else
      body = (
        <EmptyState
          title={t("Could not open this link")}
          icon={<Link2Off size={28} />}
        >
          <span className="drive-muted">{errorMessage(e)}</span>
        </EmptyState>
      );
  } else if (info.data.isDir)
    body = <FolderShare token={token} access={access} info={info.data} />;
  else body = <FileShare token={token} access={access} info={info.data} />;

  return (
    <div className="share-page">
      <header className="share-head">
        <span className="brand-mark">X</span>
        <span>X Console</span>
        <small className="drive-muted">{t("Shared files")}</small>
      </header>
      <main className="share-card">{body}</main>
    </div>
  );
}

function CodeForm({
  token,
  onUnlocked,
}: {
  token: string;
  onUnlocked: (access: string) => void;
}) {
  const t = useT();
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!code.trim()) return;
    setBusy(true);
    setError("");
    try {
      const r = await unwrap(
        driveApi.POST("/public/shares/{token}/unlock", {
          params: { path: { token } },
          body: { code: code.trim() },
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
    <form className="share-code" onSubmit={(e) => void submit(e)}>
      <KeyRound size={28} className="drive-muted" />
      <strong>{t("Enter the access code")}</strong>
      <span className="drive-muted">分享的人会把提取码一起发给你。</span>
      <input
        className="xc-input"
        autoFocus
        value={code}
        maxLength={8}
        aria-label={t("Access code")}
        onChange={(e) => setCode(e.target.value)}
      />
      {error && <small className="share-error">{error}</small>}
      <button type="submit" className="xc-btn primary" disabled={busy}>
        {t("Open")}
      </button>
    </form>
  );
}

type Info = {
  name: string;
  isDir: boolean;
  size: number;
  mime?: string;
  updatedAt?: string;
  expiresAt?: string;
  downloadsLeft?: number;
};

function Meta({ info }: { info: Info }) {
  const t = useT();
  const language = useLanguage();
  return (
    <small className="drive-muted share-meta">
      {formatBytes(info.size)}
      {info.updatedAt && ` · ${relativeTime(info.updatedAt, language)}`}
      {info.expiresAt &&
        ` · ${relativeTime(info.expiresAt, language)}${t("expires")}`}
      {info.downloadsLeft != null &&
        ` · ${t("Downloads left")} ${info.downloadsLeft}`}
    </small>
  );
}

function FileShare({
  token,
  access,
  info,
}: {
  token: string;
  access: string;
  info: Info;
}) {
  const t = useT();
  const kind = fileKind({ isDir: false, name: info.name, mime: info.mime });
  const src = publicUrl(token, "content", { access, inline: true });
  return (
    <div className="share-file">
      <div className="share-title">
        <FileIcon
          item={{ isDir: false, name: info.name, mime: info.mime }}
          size={22}
        />
        <strong title={info.name}>{info.name}</strong>
      </div>
      <Meta info={info} />
      {kind === "image" ? (
        <img className="share-preview" src={src} alt={info.name} />
      ) : kind === "video" ? (
        <video className="share-preview" src={src} controls playsInline />
      ) : kind === "audio" ? (
        <audio src={src} controls />
      ) : kind === "pdf" ? (
        <iframe
          className="share-preview share-pdf"
          src={src}
          title={info.name}
        />
      ) : null}
      <a
        className="xc-btn primary"
        href={publicUrl(token, "content", { access })}
        download={info.name}
      >
        <Download size={14} /> {t("Download")}
      </a>
    </div>
  );
}

function FolderShare({
  token,
  access,
  info,
}: {
  token: string;
  access: string;
  info: Info;
}) {
  const t = useT();
  const language = useLanguage();
  const [folder, setFolder] = useState<number | undefined>(undefined);
  const items = useQuery({
    queryKey: ["share", token, access, "items", folder ?? 0],
    queryFn: () =>
      unwrap(
        driveApi.GET("/public/shares/{token}/items", {
          params: {
            path: { token },
            query: { t: access || undefined, folder },
          },
        }),
      ),
    retry: false,
  });
  return (
    <div className="share-folder">
      <div className="share-title">
        <FolderOpen size={22} className="drive-icon folder" />
        <strong title={info.name}>{info.name}</strong>
        <span className="xc-spacer" />
        <a
          className="xc-btn small primary"
          href={publicUrl(token, "zip", { access })}
          download={`${info.name}.zip`}
        >
          <Download size={14} />
          <span className="drive-btn-text">{t("Download all")}</span>
        </a>
      </div>
      <Meta info={info} />
      <nav className="drive-crumbs small" aria-label={t("Path")}>
        <button type="button" onClick={() => setFolder(undefined)}>
          {info.name}
        </button>
        {items.data?.path.map((p) => (
          <span key={p.id}>
            <ChevronRight size={12} />
            <button type="button" onClick={() => setFolder(p.id)}>
              {p.name}
            </button>
          </span>
        ))}
      </nav>
      {items.isPending ? (
        <Loading />
      ) : items.isError ? (
        <p className="share-error">{errorMessage(items.error)}</p>
      ) : items.data.items.length === 0 ? (
        <p className="drive-muted">{t("This folder is empty")}</p>
      ) : (
        <ul className="share-list">
          {items.data.items.map((i) => (
            <li key={i.id}>
              {i.isDir ? (
                <button type="button" onClick={() => setFolder(i.id)}>
                  <FileIcon item={i} />
                  <span>{i.name}</span>
                </button>
              ) : (
                <a
                  href={publicUrl(token, "content", { access, item: i.id })}
                  download={i.name}
                >
                  <FileIcon item={i} />
                  <span>{i.name}</span>
                </a>
              )}
              <small className="drive-muted">
                {i.isDir ? t("Folder") : formatBytes(i.size)} ·{" "}
                {relativeTime(i.updatedAt, language)}
              </small>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

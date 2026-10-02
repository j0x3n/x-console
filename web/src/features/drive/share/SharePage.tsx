import { useEffect, useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Download,
  FolderOpen,
  KeyRound,
  LayoutGrid,
  Link2Off,
  List,
} from "lucide-react";
import { ApiError, errorMessage, unwrap } from "../../../api/client";
import { EmptyState, Loading } from "../../../components/ui/States";
import { Segmented } from "../../../components/ui/Toolbar";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatBytes, relativeTime } from "../../../lib/time";
import { driveApi } from "../api";
import FileIcon from "../components/FileIcon";
import SharePreview from "./SharePreview";
import "../i18n";
import "../drive.css";

/*
 * 分享页（B31、B75）：https://<面板>/s/<token>，不用登录。
 * - 有密码时先输密码，服务端发一个访问令牌，存在 sessionStorage。
 * - 文件：顶部一行信息和“下载”，下面是预览。图片、视频、音频、PDF、文本和 Markdown 都能直接看。
 * - 文件夹：列表或网格，点文件在页面里预览，可以上一个、下一个；每行有下载按钮，也能整个打包下载。
 * - 预览不算下载次数，点“下载”才算。
 */

const KEY = (token: string) => `xc.share.${token}`;
const VIEW_KEY = "xc.share.view";

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
  part: "content" | "zip" | "thumbnail",
  opts: {
    access?: string;
    item?: number;
    inline?: boolean;
    preview?: boolean;
  } = {},
) {
  const q = new URLSearchParams();
  if (opts.access) q.set("t", opts.access);
  if (opts.item) q.set("item", String(opts.item));
  if (opts.inline) q.set("inline", "1");
  if (opts.preview) q.set("preview", "true");
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
    else if (e instanceof ApiError && (e.status === 404 || e.status === 410))
      body = (
        <EmptyState
          title={t("This link does not work")}
          icon={<Link2Off size={28} />}
        >
          <span className="drive-muted">
            {e.status === 410
              ? "下载次数已经用完了。"
              : "链接可能已经过期、被取消，或者文件已经删掉了。"}
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
      <strong>{t("This link needs a password")}</strong>
      <span className="drive-muted">分享的人会把密码一起发给你。</span>
      <input
        className="xc-input"
        type="password"
        autoFocus
        autoComplete="off"
        value={code}
        maxLength={32}
        aria-label={t("Password")}
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
  return (
    <div className="share-file">
      <div className="share-top">
        <div className="share-top-text">
          <div className="share-title">
            <FileIcon
              item={{ isDir: false, name: info.name, mime: info.mime }}
              size={22}
            />
            <strong title={info.name}>{info.name}</strong>
          </div>
          <Meta info={info} />
        </div>
        <a
          className="xc-btn primary"
          href={publicUrl(token, "content", { access })}
          download={info.name}
        >
          <Download size={14} /> {t("Download")}
        </a>
      </div>
      <SharePreview
        file={{ name: info.name, mime: info.mime, size: info.size }}
        src={publicUrl(token, "content", { access, preview: true })}
      />
    </div>
  );
}

type Item = {
  id: number;
  name: string;
  isDir: boolean;
  size: number;
  mime?: string;
  updatedAt: string;
  thumbnail?: boolean;
};

function readView(): "list" | "grid" | null {
  try {
    const v = localStorage.getItem(VIEW_KEY);
    return v === "list" || v === "grid" ? v : null;
  } catch {
    return null;
  }
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
  const [viewing, setViewing] = useState<number | null>(null);
  const [view, setView] = useState<"list" | "grid" | null>(readView);
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
  const list: Item[] = items.data?.items ?? [];
  const files = list.filter((i) => !i.isDir);
  const thumbs = files.filter((i) => i.thumbnail).length;
  // 没选过时，图片多的文件夹默认用网格。
  const mode =
    view ?? (thumbs >= 4 && thumbs * 2 >= list.length ? "grid" : "list");
  const setMode = (v: "list" | "grid") => {
    setView(v);
    try {
      localStorage.setItem(VIEW_KEY, v);
    } catch {
      /* 存不了就算了 */
    }
  };
  const open = (id?: number) => {
    setViewing(null);
    setFolder(id);
  };
  const current = files.find((f) => f.id === viewing);
  const index = current ? files.indexOf(current) : -1;
  const download = (i: Item) =>
    publicUrl(token, "content", { access, item: i.id });

  // 预览时左右方向键切换文件，Esc 返回列表。
  useEffect(() => {
    if (!current) return;
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el?.closest("input, textarea, video, audio, .cm-editor")) return;
      if (e.key === "ArrowLeft" && index > 0) setViewing(files[index - 1].id);
      else if (e.key === "ArrowRight" && index < files.length - 1)
        setViewing(files[index + 1].id);
      else if (e.key === "Escape") setViewing(null);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  });

  if (current)
    return (
      <div className="share-folder">
        <div className="share-viewer-bar">
          <button
            type="button"
            className="xc-btn ghost small"
            onClick={() => setViewing(null)}
          >
            <ArrowLeft size={14} /> {t("Back to list")}
          </button>
          <span className="xc-spacer" />
          <button
            type="button"
            className="xc-btn ghost small"
            disabled={index <= 0}
            title={t("Previous file")}
            aria-label={t("Previous file")}
            onClick={() => setViewing(files[index - 1].id)}
          >
            <ChevronLeft size={15} />
          </button>
          <small className="drive-muted">
            {index + 1} / {files.length}
          </small>
          <button
            type="button"
            className="xc-btn ghost small"
            disabled={index >= files.length - 1}
            title={t("Next file")}
            aria-label={t("Next file")}
            onClick={() => setViewing(files[index + 1].id)}
          >
            <ChevronRight size={15} />
          </button>
        </div>
        <div className="share-top">
          <div className="share-top-text">
            <div className="share-title">
              <FileIcon item={current} size={22} />
              <strong title={current.name}>{current.name}</strong>
            </div>
            <small className="drive-muted share-meta">
              {formatBytes(current.size)} ·{" "}
              {relativeTime(current.updatedAt, language)}
            </small>
          </div>
          <a
            className="xc-btn primary"
            href={download(current)}
            download={current.name}
          >
            <Download size={14} /> {t("Download")}
          </a>
        </div>
        <SharePreview
          key={current.id}
          file={current}
          src={publicUrl(token, "content", {
            access,
            item: current.id,
            preview: true,
          })}
        />
      </div>
    );

  return (
    <div className="share-folder">
      <div className="share-top">
        <div className="share-top-text">
          <div className="share-title">
            <FolderOpen size={22} className="drive-icon folder" />
            <strong title={info.name}>{info.name}</strong>
          </div>
          <Meta info={info} />
        </div>
        <a
          className="xc-btn primary"
          href={publicUrl(token, "zip", { access })}
          download={`${info.name}.zip`}
        >
          <Download size={14} /> {t("Download all")}
        </a>
      </div>
      <div className="share-folder-bar">
        {folder != null && (
          <nav className="drive-crumbs small" aria-label={t("Path")}>
            <button type="button" onClick={() => open(undefined)}>
              {info.name}
            </button>
            {items.data?.path.map((p) => (
              <span key={p.id}>
                <ChevronRight size={12} />
                <button type="button" onClick={() => open(p.id)}>
                  {p.name}
                </button>
              </span>
            ))}
          </nav>
        )}
        <span className="xc-spacer" />
        <Segmented
          label={t("View")}
          value={mode}
          onChange={setMode}
          options={[
            {
              value: "list",
              label: t("List view"),
              icon: List,
              iconOnly: true,
            },
            {
              value: "grid",
              label: t("Grid view"),
              icon: LayoutGrid,
              iconOnly: true,
            },
          ]}
        />
      </div>
      {items.isPending ? (
        <Loading />
      ) : items.isError ? (
        <p className="share-error">{errorMessage(items.error)}</p>
      ) : list.length === 0 ? (
        <p className="drive-muted">{t("This folder is empty")}</p>
      ) : mode === "grid" ? (
        <ul className="share-grid">
          {list.map((i) => (
            <li key={i.id}>
              <button
                type="button"
                title={i.name}
                onClick={() => (i.isDir ? open(i.id) : setViewing(i.id))}
              >
                <span className="share-grid-thumb">
                  {i.thumbnail ? (
                    <img
                      src={publicUrl(token, "thumbnail", {
                        access,
                        item: i.id,
                      })}
                      alt=""
                      loading="lazy"
                    />
                  ) : (
                    <FileIcon item={i} size={30} />
                  )}
                </span>
                <span className="share-grid-name">{i.name}</span>
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <ul className="share-list">
          {list.map((i) => (
            <li key={i.id}>
              <button
                type="button"
                onClick={() => (i.isDir ? open(i.id) : setViewing(i.id))}
              >
                <FileIcon item={i} />
                <span>{i.name}</span>
              </button>
              <small className="drive-muted">
                {i.isDir ? t("Folder") : formatBytes(i.size)} ·{" "}
                {relativeTime(i.updatedAt, language)}
              </small>
              {i.isDir ? (
                <span className="share-list-gap" />
              ) : (
                <a
                  className="xc-btn ghost small"
                  href={download(i)}
                  download={i.name}
                  title={t("Download")}
                  aria-label={`${t("Download")} ${i.name}`}
                >
                  <Download size={14} />
                </a>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

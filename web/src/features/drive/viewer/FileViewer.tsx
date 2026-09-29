import { useEffect, useRef, useState } from "react";
import {
  ChevronLeft,
  ChevronRight,
  Download,
  Maximize2,
  Minimize2,
  SquarePen,
  X,
} from "lucide-react";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { useLanguage, useT } from "../../../contexts/LanguageContext";
import { formatBytes, relativeTime } from "../../../lib/time";
import { contentUrl, type DriveItem } from "../api";
import FileIcon from "../components/FileIcon";
import ImageView from "./ImageView";
import { canEdit, neighbor, viewerKind } from "./kind";
import LogView from "./LogView";
import TextView from "./TextView";

/** 在全屏状态下先退出全屏，不然确认框显示不出来。 */
async function leaveFullscreen() {
  if (document.fullscreenElement)
    await document.exitFullscreen().catch(() => undefined);
}

/** 焦点在输入框、编辑器、播放器上时，左右键留给它们。 */
function ownsArrows(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target.isContentEditable ||
    /^(INPUT|TEXTAREA|SELECT|VIDEO|AUDIO)$/.test(target.tagName)
  );
}

/**
 * 统一的文件查看器（B31 预览）。
 * - 图片、视频、音频、PDF、文本、Markdown、日志各有自己的显示方式，其他显示文件信息和下载。
 * - 右上角全屏；Esc 关闭（全屏时 Esc 先退出全屏）。
 * - 左右键或两边的按钮切换同一个文件夹里的文件。
 * - 手机上直接铺满屏幕。
 */
export default function FileViewer({
  items,
  initialId,
  initialEdit = false,
  onClose,
}: {
  /** 同一个文件夹里能切换的文件，按列表顺序。 */
  items: DriveItem[];
  initialId: number;
  initialEdit?: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const [id, setId] = useState(initialId);
  const [editing, setEditing] = useState(initialEdit);
  const [fullscreen, setFullscreen] = useState(false);
  const [saved, setSaved] = useState<DriveItem | null>(null);
  const dirty = useRef(false);
  const root = useRef<HTMLDivElement>(null);

  const listed = items.find((i) => i.id === id);
  const item = saved?.id === id ? saved : listed;
  const index = items.findIndex((i) => i.id === id);

  const confirmLeave = async () => {
    if (!dirty.current) return true;
    await leaveFullscreen();
    return confirmAction({
      title: t("Discard unsaved changes?"),
      description: "没保存的改动会丢掉。",
    });
  };

  const close = async () => {
    if (!(await confirmLeave())) return;
    await leaveFullscreen();
    onClose();
  };

  const go = async (step: 1 | -1) => {
    const next = neighbor(items, id, step);
    if (next < 0 || !(await confirmLeave())) return;
    dirty.current = false;
    setEditing(false);
    setId(items[next].id);
  };

  const toggleFullscreen = () => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void root.current?.requestFullscreen?.().catch(() => undefined);
  };

  useEffect(() => {
    const onChange = () =>
      setFullscreen(document.fullscreenElement === root.current);
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, []);

  // 列表里的文件被删掉（比如别的标签页删的）就关掉。
  useEffect(() => {
    if (!listed) onClose();
  }, [listed]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // 上面还有别的弹窗（比如确认框）时不处理。
      if (document.querySelector(".modal-backdrop:not(.drive-viewer-backdrop)"))
        return;
      if (e.key === "Escape" && !document.fullscreenElement) {
        e.preventDefault();
        void close();
      } else if (
        (e.key === "ArrowLeft" || e.key === "ArrowRight") &&
        !e.altKey &&
        !e.metaKey &&
        !e.ctrlKey &&
        !ownsArrows(e.target)
      ) {
        e.preventDefault();
        void go(e.key === "ArrowLeft" ? -1 : 1);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  });

  // 有没保存的改动时，关页面前浏览器会提示。
  useEffect(() => {
    const onUnload = (e: BeforeUnloadEvent) => {
      if (dirty.current) e.preventDefault();
    };
    window.addEventListener("beforeunload", onUnload);
    return () => window.removeEventListener("beforeunload", onUnload);
  }, []);

  if (!item) return null;
  const kind = viewerKind(item);
  const editable = canEdit(item);
  const showText =
    (editing && editable) || kind === "text" || kind === "markdown";
  // 两边的大箭头只放在图片、音频这类不会被挡住内容的地方；标题栏里一直有小箭头。
  const sideNav =
    !showText && (kind === "image" || kind === "audio" || kind === "other");
  const src = contentUrl(item.id, true);
  const hasPrev = index > 0;
  const hasNext = index >= 0 && index < items.length - 1;

  return (
    <div
      className="modal-backdrop drive-viewer-backdrop"
      onMouseDown={(e) => e.target === e.currentTarget && void close()}
    >
      <div
        ref={root}
        className={`drive-viewer is-${kind}${fullscreen ? " is-fullscreen" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label={item.name}
      >
        <header className="drive-viewer-head">
          <FileIcon item={item} />
          <div className="drive-viewer-title">
            <h2 title={item.name}>{item.name}</h2>
            <small>
              {formatBytes(item.size)} ·{" "}
              {relativeTime(item.updatedAt, language)}
              {items.length > 1 && index >= 0 && (
                <>
                  {" "}
                  · {index + 1} / {items.length}
                </>
              )}
            </small>
          </div>
          <div className="drive-viewer-actions">
            {items.length > 1 && (
              <span className="drive-viewer-step">
                <button
                  type="button"
                  className="xc-btn ghost small"
                  title={t("Previous file")}
                  aria-label={t("Previous file")}
                  disabled={!hasPrev}
                  onClick={() => void go(-1)}
                >
                  <ChevronLeft size={15} />
                </button>
                <button
                  type="button"
                  className="xc-btn ghost small"
                  title={t("Next file")}
                  aria-label={t("Next file")}
                  disabled={!hasNext}
                  onClick={() => void go(1)}
                >
                  <ChevronRight size={15} />
                </button>
              </span>
            )}
            {editable && !editing && (
              <button
                type="button"
                className="xc-btn small"
                title={t("Edit")}
                aria-label={t("Edit")}
                onClick={() => setEditing(true)}
              >
                <SquarePen size={14} />
                <span className="drive-btn-text">{t("Edit")}</span>
              </button>
            )}
            <a
              className="xc-btn ghost small"
              href={contentUrl(item.id)}
              download={item.name}
              title={t("Download")}
              aria-label={t("Download")}
            >
              <Download size={15} />
            </a>
            <button
              type="button"
              className="xc-btn ghost small drive-viewer-fs"
              title={fullscreen ? t("Exit full screen") : t("Full screen")}
              aria-label={fullscreen ? t("Exit full screen") : t("Full screen")}
              onClick={toggleFullscreen}
            >
              {fullscreen ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
            </button>
            <button
              type="button"
              className="xc-btn ghost small"
              title={t("Close")}
              aria-label={t("Close")}
              onClick={() => void close()}
            >
              <X size={16} />
            </button>
          </div>
        </header>

        <div className="drive-viewer-body">
          {showText ? (
            <TextView
              key={item.id}
              item={item}
              markdown={kind === "markdown"}
              editing={editing && editable}
              onEditingChange={setEditing}
              onDirtyChange={(d) => {
                dirty.current = d;
              }}
              onSaved={setSaved}
            />
          ) : kind === "image" ? (
            <ImageView key={item.id} src={src} alt={item.name} />
          ) : kind === "video" ? (
            <video key={item.id} src={src} controls playsInline autoPlay />
          ) : kind === "audio" ? (
            <div className="drive-viewer-center">
              <FileIcon item={item} size={48} />
              <audio key={item.id} src={src} controls autoPlay />
            </div>
          ) : kind === "pdf" ? (
            <iframe key={item.id} src={src} title={item.name} />
          ) : kind === "log" ? (
            <LogView key={item.id} item={item} />
          ) : (
            <div className="drive-viewer-center">
              <FileIcon item={item} size={48} />
              <strong>{item.name}</strong>
              <span className="drive-muted">
                {item.mime || t("Unknown type")} · {formatBytes(item.size)}
              </span>
              <span className="drive-muted">这种文件不能在这里预览。</span>
              <a
                className="xc-btn primary"
                href={contentUrl(item.id)}
                download={item.name}
              >
                <Download size={14} /> {t("Download")}
              </a>
            </div>
          )}

          {sideNav && hasPrev && (
            <button
              type="button"
              className="drive-viewer-nav prev"
              title={t("Previous file")}
              aria-label={t("Previous file")}
              onClick={() => void go(-1)}
            >
              <ChevronLeft size={20} />
            </button>
          )}
          {sideNav && hasNext && (
            <button
              type="button"
              className="drive-viewer-nav next"
              title={t("Next file")}
              aria-label={t("Next file")}
              onClick={() => void go(1)}
            >
              <ChevronRight size={20} />
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Copy, Download, Share } from "lucide-react";
import { domToBlob } from "modern-screenshot";
import Dialog from "../../../components/ui/Dialog";
import Markdown from "../../../components/markdown/Markdown";
import { Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { errorMessage } from "../../../api/client";
import { noteBgClass } from "../noteColors";
import {
  exportDate,
  hasWideTable,
  imageFileName,
  pickScale,
  readParts,
  readTableMode,
  saveParts,
  saveTableMode,
  SHEET_WIDTH,
  WIDE_SHEET_WIDTH,
  wideSheetWidth,
  type ImageParts,
  type TableMode,
} from "../imageExport";

/** 读不到的图片换成这张灰色方块。 */
const PLACEHOLDER = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" width="352" height="120"><rect width="100%" height="100%" rx="6" fill="#e4e4e7"/><text x="50%" y="50%" fill="#71717a" font-size="14" font-family="sans-serif" text-anchor="middle" dominant-baseline="middle">图片无法导出</text></svg>',
)}`;

const PART_LABELS: { key: keyof ImageParts; label: string }[] = [
  { key: "title", label: "Note title" },
  { key: "date", label: "Date" },
  { key: "tags", label: "Tags" },
  { key: "hidden", label: "Hidden block content" },
  { key: "brand", label: "X Console at the bottom" },
];

interface Result {
  url: string;
  blob: Blob;
  failed: number;
}

type Status =
  | { kind: "working" }
  | { kind: "done"; result: Result }
  | { kind: "tooLong" }
  | { kind: "error"; message: string };

/** 等页面里的图片都加载完或失败，最多等 10 秒。读不到的换成灰色方块，返回张数。 */
async function settleImages(root: HTMLElement): Promise<number> {
  const images = Array.from(root.querySelectorAll("img"));
  await Promise.all(
    images.map((img) =>
      img.complete
        ? Promise.resolve()
        : new Promise<void>((resolve) => {
            const done = () => resolve();
            img.addEventListener("load", done, { once: true });
            img.addEventListener("error", done, { once: true });
            window.setTimeout(done, 10_000);
          }),
    ),
  );
  for (const img of images)
    if (img.naturalWidth === 0 && !img.dataset.failed) {
      img.dataset.failed = "1";
      img.src = PLACEHOLDER;
      await img.decode().catch(() => undefined);
    }
  return root.querySelectorAll("img[data-failed]").length;
}

const coarse = () =>
  typeof window.matchMedia === "function" &&
  window.matchMedia("(pointer: coarse)").matches;

function canShareFiles(): boolean {
  try {
    const file = new File([""], "a.png", { type: "image/png" });
    return (
      coarse() &&
      typeof navigator.canShare === "function" &&
      navigator.canShare({ files: [file] })
    );
  } catch {
    return false;
  }
}

const canCopyImage = () =>
  typeof ClipboardItem !== "undefined" &&
  typeof navigator.clipboard?.write === "function";

/**
 * 笔记导出图片（B242）。上面选范围和要放的内容，下面是生成好的图片预览。
 * 在页面外渲染一份 400px 宽的排版，用 modern-screenshot 画成 PNG。
 * 有 4 列以上的表格时可以选画成卡片，或者保持表格、图片跟着加宽（B246）。
 */
export default function ImageExportDialog({
  note,
  selection,
  onClose,
}: {
  note: {
    title: string;
    body: string;
    tags: string[];
    color?: string;
    updatedAt: string;
  };
  /** 打开弹窗前选中的 Markdown 原文，没选中是空字符串 */
  selection: string;
  onClose: () => void;
}) {
  const t = useT();
  const [scope, setScope] = useState<"all" | "selection">(
    selection ? "selection" : "all",
  );
  const [parts, setParts] = useState<ImageParts>(readParts);
  const [tableMode, setTableMode] = useState<TableMode>(readTableMode);
  const [status, setStatus] = useState<Status>({ kind: "working" });
  const sheetRef = useRef<HTMLDivElement>(null);
  const urlRef = useRef<string | null>(null);
  const [share] = useState(canShareFiles);
  const [copy] = useState(canCopyImage);

  const source = scope === "selection" ? selection : note.body;
  const date = exportDate(note.updatedAt);
  const wideTable = useMemo(() => hasWideTable(source), [source]);
  const widen = wideTable && tableMode === "wide";

  const toggle = (key: keyof ImageParts) =>
    setParts((p) => {
      const next = { ...p, [key]: !p[key] };
      saveParts(next);
      return next;
    });

  const pickTableMode = (mode: TableMode) => {
    setTableMode(mode);
    saveTableMode(mode);
  };

  useEffect(() => {
    let stale = false;
    setStatus({ kind: "working" });
    const timer = window.setTimeout(async () => {
      const sheet = sheetRef.current;
      if (!sheet) return;
      try {
        await document.fonts?.ready;
        let failed = await settleImages(sheet);
        // B246 加宽表格：先按最宽排一次，量出最宽的表格，再定排版宽度
        let width = SHEET_WIDTH;
        sheet.style.width = "";
        if (widen) {
          sheet.style.width = `${WIDE_SHEET_WIDTH}px`;
          const tables =
            sheet.querySelectorAll<HTMLElement>(".xc-md-table table");
          width = wideSheetWidth(
            Math.max(0, ...Array.from(tables, (t) => t.offsetWidth)),
          );
          sheet.style.width = `${width}px`;
        }
        const scale = pickScale(sheet.offsetHeight, width);
        if (stale) return;
        if (scale === null) {
          setStatus({ kind: "tooLong" });
          return;
        }
        const blob = await domToBlob(sheet, {
          scale,
          width,
          type: "image/png",
          fetch: {
            placeholderImage: () => {
              failed++;
              return PLACEHOLDER;
            },
          },
        });
        if (stale) return;
        if (urlRef.current) URL.revokeObjectURL(urlRef.current);
        const url = URL.createObjectURL(blob);
        urlRef.current = url;
        setStatus({ kind: "done", result: { url, blob, failed } });
      } catch (error) {
        if (!stale) setStatus({ kind: "error", message: errorMessage(error) });
      }
    }, 250);
    return () => {
      stale = true;
      window.clearTimeout(timer);
    };
  }, [source, parts, widen, note.title, note.color, note.tags, date]);

  useEffect(
    () => () => {
      if (urlRef.current) URL.revokeObjectURL(urlRef.current);
    },
    [],
  );

  const result = status.kind === "done" ? status.result : null;
  const fileName = imageFileName(note.title, new Date(), t("Note"));

  const warnFailed = (r: Result) => {
    if (r.failed > 0)
      toast(t("{n} images were not exported").replace("{n}", String(r.failed)));
  };
  const save = (r: Result) => {
    const a = document.createElement("a");
    a.href = r.url;
    a.download = fileName;
    document.body.appendChild(a);
    a.click();
    a.remove();
    warnFailed(r);
  };
  const shareImage = async (r: Result) => {
    try {
      await navigator.share({
        files: [new File([r.blob], fileName, { type: "image/png" })],
      });
      warnFailed(r);
    } catch (error) {
      if ((error as Error)?.name !== "AbortError")
        toast({ message: errorMessage(error), tone: "error" });
    }
  };
  const copyImage = async (r: Result) => {
    try {
      await navigator.clipboard.write([
        new ClipboardItem({ "image/png": r.blob }),
      ]);
      toast(t("Image copied"));
      warnFailed(r);
    } catch {
      toast({ message: t("Could not copy the image"), tone: "error" });
    }
  };

  let preview;
  if (status.kind === "working") preview = <Loading />;
  else if (status.kind === "tooLong")
    preview = (
      <p className="xc-error-text">
        {t("Too long for one image. Select a part and export again.")}
      </p>
    );
  else if (status.kind === "error")
    preview = <p className="xc-error-text">{status.message}</p>;
  else
    preview = (
      <img
        className="notes-img-preview-img"
        src={status.result.url}
        alt={t("Image preview")}
      />
    );

  return (
    <Dialog open onClose={onClose} title={t("Export image")} wide>
      <div className="notes-img-export">
        <div className="notes-img-options">
          <div className="xc-tabs" role="group" aria-label={t("Range")}>
            <button
              type="button"
              className={scope === "all" ? "active" : ""}
              aria-pressed={scope === "all"}
              onClick={() => setScope("all")}
            >
              {t("Whole note")}
            </button>
            <button
              type="button"
              className={scope === "selection" ? "active" : ""}
              aria-pressed={scope === "selection"}
              disabled={!selection}
              title={selection ? undefined : t("Select some text first")}
              onClick={() => setScope("selection")}
            >
              {t("Selected part")}
            </button>
          </div>
          {wideTable && (
            <div className="xc-tabs" role="group" aria-label={t("Table")}>
              <button
                type="button"
                className={tableMode === "cards" ? "active" : ""}
                aria-pressed={tableMode === "cards"}
                title={t("Each row becomes a card. Easy to read on a phone.")}
                onClick={() => pickTableMode("cards")}
              >
                {t("Cards")}
              </button>
              <button
                type="button"
                className={tableMode === "wide" ? "active" : ""}
                aria-pressed={tableMode === "wide"}
                title={t("Keep the table. The image gets wider with it.")}
                onClick={() => pickTableMode("wide")}
              >
                {t("Wide table")}
              </button>
            </div>
          )}
          <div className="notes-img-parts">
            {PART_LABELS.map((x) => (
              <label key={x.key} className="xc-check">
                <input
                  type="checkbox"
                  checked={parts[x.key]}
                  onChange={() => toggle(x.key)}
                />
                <span>{t(x.label)}</span>
              </label>
            ))}
          </div>
        </div>
        <div className="notes-img-preview" aria-live="polite">
          {preview}
        </div>
        {result && result.failed > 0 && (
          <small className="xc-muted">
            {t("{n} images were not exported").replace(
              "{n}",
              String(result.failed),
            )}
          </small>
        )}
      </div>
      <div className="xc-dialog-actions">
        <span className="xc-spacer" />
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Close")}
        </button>
        {copy && !share && (
          <button
            type="button"
            className="xc-btn"
            disabled={!result}
            onClick={() => result && void copyImage(result)}
          >
            <Copy size={14} /> {t("Copy image")}
          </button>
        )}
        <button
          type="button"
          className={`xc-btn${share ? "" : " primary"}`}
          disabled={!result}
          onClick={() => result && save(result)}
        >
          <Download size={14} /> {t("Save image")}
        </button>
        {share && (
          <button
            type="button"
            className="xc-btn primary"
            disabled={!result}
            onClick={() => result && void shareImage(result)}
          >
            <Share size={14} /> {t("Share")}
          </button>
        )}
      </div>
      {createPortal(
        <div className="notes-img-stage" aria-hidden>
          <div
            ref={sheetRef}
            className={`notes-img-sheet${widen ? " wide" : ""} ${noteBgClass(note.color)}`}
          >
            {parts.title && note.title.trim() && (
              <div className="notes-img-title">{note.title}</div>
            )}
            {((parts.date && date) || (parts.tags && note.tags.length > 0)) && (
              <div className="notes-img-meta">
                {parts.date && date && <span>{date}</span>}
                {parts.tags &&
                  note.tags.map((tag) => (
                    <span key={tag} className="notes-img-tag">
                      #{tag}
                    </span>
                  ))}
              </div>
            )}
            <Markdown
              source={source}
              snapshot={{ showHidden: parts.hidden, tables: tableMode }}
            />
            {parts.brand && <div className="notes-img-brand">X Console</div>}
          </div>
        </div>,
        document.body,
      )}
    </Dialog>
  );
}

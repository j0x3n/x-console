import { useEffect, useState } from "react";
import { Download } from "lucide-react";
import { apiFetch, errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { formatBytes } from "../../../lib/time";
import { contentUrl, type DriveItem } from "../api";
import { fileKind, TEXT_PREVIEW_BYTES } from "../logic";

export default function PreviewDialog({
  item,
  onClose,
}: {
  item: DriveItem;
  onClose: () => void;
}) {
  const t = useT();
  const kind = fileKind(item);
  const src = contentUrl(item.id, true);
  return (
    <Dialog
      open
      wide
      onClose={onClose}
      title={item.name}
      description={formatBytes(item.size)}
    >
      <div className={`drive-preview ${kind}`}>
        {kind === "image" && <img src={src} alt={item.name} />}
        {kind === "pdf" && <iframe src={src} title={item.name} />}
        {kind === "video" && <video src={src} controls playsInline />}
        {kind === "audio" && <audio src={src} controls />}
        {kind === "text" && <TextPreview item={item} />}
      </div>
      <div className="xc-dialog-actions">
        <a className="xc-btn" href={contentUrl(item.id)} download={item.name}>
          <Download size={14} /> {t("Download")}
        </a>
        <button type="button" className="xc-btn primary" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

/** 文本和代码只取前 1 MB。 */
function TextPreview({ item }: { item: DriveItem }) {
  const [text, setText] = useState<string | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let alive = true;
    apiFetch(`/drive/items/${item.id}/content?inline=1`, {
      headers: { Range: `bytes=0-${TEXT_PREVIEW_BYTES - 1}` },
    })
      .then((r) => r.text())
      .then((s) => alive && setText(s))
      .catch((e) => alive && setError(errorMessage(e)));
    return () => {
      alive = false;
    };
  }, [item.id, item.updatedAt]);
  if (error) return <p className="xc-error-text">{error}</p>;
  if (text == null) return <Loading />;
  return (
    <>
      <pre>{text}</pre>
      {item.size > TEXT_PREVIEW_BYTES && (
        <p className="drive-muted">只显示了前 1 MB，完整内容请下载。</p>
      )}
    </>
  );
}

import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { ChevronLeft, ChevronRight, ExternalLink, X } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";

export interface LightboxImage {
  src: string;
  alt?: string;
}

/*
 * 页内看大图（B54）。点背景或按 Esc 关闭，多张图时左右键切换。
 * 不在新窗口打开，右上角留一个“打开原图”。
 */
export default function ImageLightbox({
  images,
  start = 0,
  onClose,
}: {
  images: LightboxImage[];
  start?: number;
  onClose: () => void;
}) {
  const t = useT();
  const [index, setIndex] = useState(start);
  const count = images.length;
  const image = images[Math.min(index, count - 1)];

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowLeft") setIndex((i) => (i - 1 + count) % count);
      else if (e.key === "ArrowRight") setIndex((i) => (i + 1) % count);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [count, onClose]);

  if (!image) return null;
  const step = (d: number) => (e: React.MouseEvent) => {
    e.stopPropagation();
    setIndex((i) => (i + d + count) % count);
  };

  return createPortal(
    <div
      className="xc-lightbox"
      role="dialog"
      aria-label={image.alt || t("Image")}
      onClick={onClose}
    >
      <div className="xc-lightbox-bar" onClick={(e) => e.stopPropagation()}>
        {count > 1 && (
          <span className="xc-lightbox-count">
            {index + 1} / {count}
          </span>
        )}
        <a
          className="xc-btn small"
          href={image.src}
          target="_blank"
          rel="noreferrer"
          title={t("Open original")}
        >
          <ExternalLink size={14} />
        </a>
        <button
          type="button"
          className="xc-btn small"
          onClick={onClose}
          aria-label={t("Close")}
          title={t("Close")}
        >
          <X size={14} />
        </button>
      </div>
      {count > 1 && (
        <button
          type="button"
          className="xc-lightbox-nav prev"
          onClick={step(-1)}
          aria-label={t("Previous image")}
        >
          <ChevronLeft size={22} />
        </button>
      )}
      <img
        src={image.src}
        alt={image.alt ?? ""}
        onClick={(e) => e.stopPropagation()}
      />
      {count > 1 && (
        <button
          type="button"
          className="xc-lightbox-nav next"
          onClick={step(1)}
          aria-label={t("Next image")}
        >
          <ChevronRight size={22} />
        </button>
      )}
    </div>,
    document.body,
  );
}

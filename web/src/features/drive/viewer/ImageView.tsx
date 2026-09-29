import { useRef, useState, type PointerEvent, type WheelEvent } from "react";
import { Maximize, ZoomIn, ZoomOut } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import { clampZoom } from "./kind";

/** 图片：滚轮或按钮缩放，放大后可以拖动，双击在原图和适应窗口之间切换。 */
export default function ImageView({ src, alt }: { src: string; alt: string }) {
  const t = useT();
  const [zoom, setZoom] = useState(1);
  const [pos, setPos] = useState({ x: 0, y: 0 });
  const drag = useRef<{ x: number; y: number; px: number; py: number } | null>(
    null,
  );

  const zoomTo = (next: number) => {
    const z = clampZoom(next);
    setZoom(z);
    if (z <= 1) setPos({ x: 0, y: 0 });
  };
  const onWheel = (e: WheelEvent) => {
    zoomTo(zoom * (e.deltaY < 0 ? 1.15 : 1 / 1.15));
  };
  const onDown = (e: PointerEvent) => {
    if (zoom <= 1) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    drag.current = { x: e.clientX, y: e.clientY, px: pos.x, py: pos.y };
  };
  const onMove = (e: PointerEvent) => {
    const d = drag.current;
    if (!d) return;
    setPos({ x: d.px + e.clientX - d.x, y: d.py + e.clientY - d.y });
  };
  const onUp = () => {
    drag.current = null;
  };

  return (
    <div className="drive-image">
      <div
        className={`drive-image-stage${zoom > 1 ? " is-zoomed" : ""}`}
        onWheel={onWheel}
        onPointerDown={onDown}
        onPointerMove={onMove}
        onPointerUp={onUp}
        onPointerCancel={onUp}
        onDoubleClick={() => zoomTo(zoom === 1 ? 2 : 1)}
      >
        <img
          src={src}
          alt={alt}
          draggable={false}
          style={{
            transform: `translate(${pos.x}px, ${pos.y}px) scale(${zoom})`,
          }}
        />
      </div>
      <div className="drive-image-tools" role="toolbar" aria-label={alt}>
        <button
          type="button"
          className="xc-btn ghost small"
          title={t("Zoom out")}
          aria-label={t("Zoom out")}
          onClick={() => zoomTo(zoom / 1.25)}
        >
          <ZoomOut size={14} />
        </button>
        <button
          type="button"
          className="xc-btn ghost small drive-image-zoom"
          title={t("Fit to window")}
          onClick={() => zoomTo(1)}
        >
          {Math.round(zoom * 100)}%
        </button>
        <button
          type="button"
          className="xc-btn ghost small"
          title={t("Zoom in")}
          aria-label={t("Zoom in")}
          onClick={() => zoomTo(zoom * 1.25)}
        >
          <ZoomIn size={14} />
        </button>
        <button
          type="button"
          className="xc-btn ghost small"
          title={t("Fit to window")}
          aria-label={t("Fit to window")}
          onClick={() => zoomTo(1)}
        >
          <Maximize size={14} />
        </button>
      </div>
    </div>
  );
}

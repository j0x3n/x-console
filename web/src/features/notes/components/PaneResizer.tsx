import { useRef, type PointerEvent } from "react";
import { useT } from "../../../contexts/LanguageContext";

/*
 * B40：笔记三栏的拖动条。拖动时改宽度，松手后由调用方保存；双击恢复默认。
 * 键盘左右键每次改 16px。
 */
export default function PaneResizer({
  className,
  width,
  min,
  max,
  onChange,
  onDone,
  onReset,
}: {
  className?: string;
  width: number;
  min: number;
  max: number;
  onChange: (width: number) => void;
  /** 松手或按键后调用，带上最终宽度 */
  onDone: (width: number) => void;
  onReset: () => void;
}) {
  const t = useT();
  const start = useRef<{ x: number; width: number; last: number } | null>(null);
  const clamp = (w: number) => Math.round(Math.min(max, Math.max(min, w)));
  return (
    <div
      className={`notes-resizer${className ? ` ${className}` : ""}`}
      role="separator"
      aria-orientation="vertical"
      aria-label={t("Drag to resize")}
      aria-valuenow={width}
      aria-valuemin={min}
      aria-valuemax={max}
      tabIndex={0}
      title={t("Drag to resize, double-click to reset")}
      onPointerDown={(e: PointerEvent<HTMLDivElement>) => {
        start.current = { x: e.clientX, width, last: width };
        e.currentTarget.setPointerCapture(e.pointerId);
      }}
      onPointerMove={(e) => {
        if (!start.current) return;
        start.current.last = clamp(
          start.current.width + e.clientX - start.current.x,
        );
        onChange(start.current.last);
      }}
      onPointerUp={() => {
        if (!start.current) return;
        const last = start.current.last;
        start.current = null;
        onDone(last);
      }}
      onDoubleClick={onReset}
      onKeyDown={(e) => {
        if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
        e.preventDefault();
        onDone(clamp(width + (e.key === "ArrowRight" ? 16 : -16)));
      }}
    />
  );
}

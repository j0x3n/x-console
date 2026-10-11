import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Layers } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { MAX_SELECTION, MIN_SELECTION } from "./cloze";
import "./i18n";
import "./cards.css";

const MakeCardDialog = lazy(() => import("./MakeCardDialog"));

export interface CardSource {
  /** 笔记标题或文章标题 */
  title: string;
  /** 站内路径（/notes/3）或网址 */
  link: string;
}

interface Pick {
  text: string;
  x: number;
  y: number;
}

type Point = { x: number; y: number } | null;

/**
 * 读出 scope 里选中的文字和按钮该放的位置。选中少于 6 个字、选在 scope 外面、
 * 或没有选中时返回 null。编辑框（textarea）里的选区和页面上的选区都认。
 */
export function readSelection(scope: HTMLElement, pointer: Point): Pick | null {
  const active = document.activeElement;
  if (active instanceof HTMLTextAreaElement && scope.contains(active)) {
    const text = active.value
      .slice(active.selectionStart, active.selectionEnd)
      .trim();
    if (text.length < MIN_SELECTION) return null;
    const box = active.getBoundingClientRect();
    return {
      text: text.slice(0, MAX_SELECTION),
      ...(pointer ?? { x: box.right - 120, y: box.top + 24 }),
    };
  }
  const sel = window.getSelection();
  if (!sel || sel.isCollapsed || sel.rangeCount === 0) return null;
  if (!scope.contains(sel.anchorNode) || !scope.contains(sel.focusNode))
    return null;
  const text = sel.toString().trim();
  if (text.length < MIN_SELECTION) return null;
  const range = sel.getRangeAt(0);
  const rect =
    typeof range.getBoundingClientRect === "function"
      ? range.getBoundingClientRect()
      : null;
  const where =
    !rect || (rect.width === 0 && rect.height === 0)
      ? (pointer ?? { x: 0, y: 0 })
      : { x: rect.left + rect.width / 2, y: rect.bottom };
  return { text: text.slice(0, MAX_SELECTION), ...where };
}

/**
 * B130：在笔记和稍后阅读里选中文字，旁边出现“做成卡片”按钮。
 * 放在要监听的区域里面，它监听自己的父元素：
 *
 *   <div className="notes-editor">… <CardFromSelection source={…} /></div>
 *
 * disabled 为真时不出按钮，比如隐藏的笔记。
 */
export default function CardFromSelection({
  source,
  disabled,
}: {
  source: CardSource;
  disabled?: boolean;
}) {
  const t = useT();
  const marker = useRef<HTMLSpanElement>(null);
  const pointer = useRef<Point>(null);
  const [pick, setPick] = useState<Pick | null>(null);
  const [text, setText] = useState<string | null>(null);

  useEffect(() => {
    const scope = marker.current?.parentElement;
    if (!scope || disabled) {
      setPick(null);
      return;
    }
    const update = () => setPick(readSelection(scope, pointer.current));
    const onPointer = (e: Event) => {
      if (e instanceof MouseEvent)
        pointer.current = { x: e.clientX, y: e.clientY + 18 };
      else if (e instanceof TouchEvent && e.changedTouches[0])
        pointer.current = {
          x: e.changedTouches[0].clientX,
          y: e.changedTouches[0].clientY + 28,
        };
      update();
    };
    const events = ["mouseup", "touchend", "keyup", "select"];
    for (const name of events) scope.addEventListener(name, onPointer);
    document.addEventListener("selectionchange", update);
    return () => {
      for (const name of events) scope.removeEventListener(name, onPointer);
      document.removeEventListener("selectionchange", update);
    };
  }, [disabled]);

  const width = typeof window === "undefined" ? 1200 : window.innerWidth;
  const height = typeof window === "undefined" ? 800 : window.innerHeight;
  return (
    <>
      <span ref={marker} hidden />
      {pick &&
        text === null &&
        createPortal(
          <button
            type="button"
            className="xc-btn small primary cards-pick"
            style={{
              left: Math.max(8, Math.min(pick.x - 50, width - 130)),
              top: Math.max(8, Math.min(pick.y, height - 44)),
            }}
            // 按下时不让选区消失
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => {
              setText(pick.text);
              setPick(null);
            }}
          >
            <Layers size={13} /> {t("Make a card")}
          </button>,
          document.body,
        )}
      {text !== null && (
        <Suspense fallback={null}>
          <MakeCardDialog
            text={text}
            source={source}
            onClose={() => setText(null)}
          />
        </Suspense>
      )}
    </>
  );
}

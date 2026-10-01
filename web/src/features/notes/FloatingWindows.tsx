import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent,
} from "react";
import { useNavigate } from "react-router";
import { Maximize2, Minus, X } from "lucide-react";
import { useT } from "../../contexts/LanguageContext";
import { useNote } from "./api";
import NoteEditor from "./components/NoteEditor";
import {
  clampBox,
  defaultBox,
  useFloatingNotes,
  type FloatBox,
} from "./floating";
import { noteTitle } from "./logic";
import "./i18n";
import "./notes.css";

/** 所有打开的笔记浮窗（B72）。后打开或后点过的在上面。 */
export default function FloatingWindows() {
  const ids = useFloatingNotes((s) => s.ids);
  return (
    <>
      {ids.map((id, i) => (
        <FloatingNote key={id} id={id} index={i} z={i} />
      ))}
    </>
  );
}

function FloatingNote({
  id,
  index,
  z,
}: {
  id: number;
  index: number;
  z: number;
}) {
  const t = useT();
  const navigate = useNavigate();
  const note = useNote(id);
  const { boxes, minimized, close, toggleMin, raise, setBox } =
    useFloatingNotes();
  const min = minimized.includes(id);
  const [box, setLocal] = useState<FloatBox>(
    () => boxes[id] ?? defaultBox(index, window.innerWidth, window.innerHeight),
  );
  const drag = useRef<{
    mode: "move" | "size";
    x: number;
    y: number;
    start: FloatBox;
  } | null>(null);

  // 窗口变小时，浮窗收回屏幕里
  useEffect(() => {
    const onResize = () =>
      setLocal((b) => clampBox(b, window.innerWidth, window.innerHeight));
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  // 笔记删了或打不开（比如隐藏后锁定）时关掉浮窗
  useEffect(() => {
    if (note.isError) close(id);
  }, [note.isError]);

  const begin = (mode: "move" | "size") => (e: PointerEvent<HTMLElement>) => {
    if (e.button !== 0) return;
    if (mode === "move" && (e.target as HTMLElement).closest("button")) return;
    e.preventDefault();
    raise(id);
    drag.current = { mode, x: e.clientX, y: e.clientY, start: box };
    e.currentTarget.setPointerCapture(e.pointerId);
  };
  const move = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current;
    if (!d) return;
    const dx = e.clientX - d.x;
    const dy = e.clientY - d.y;
    const next =
      d.mode === "move"
        ? { ...d.start, x: d.start.x + dx, y: d.start.y + dy }
        : { ...d.start, w: d.start.w + dx, h: d.start.h + dy };
    setLocal(clampBox(next, window.innerWidth, window.innerHeight));
  };
  const end = () => {
    if (!drag.current) return;
    drag.current = null;
    setBox(id, box);
  };

  const title = note.data
    ? noteTitle(note.data.title, note.data.body) || t("Untitled note")
    : t("Loading");
  return (
    <section
      className={`notes-float${min ? " min" : ""}`}
      style={
        {
          "--float-x": `${box.x}px`,
          "--float-y": `${box.y}px`,
          "--float-w": `${box.w}px`,
          "--float-h": `${box.h}px`,
          zIndex: 40 + z,
        } as CSSProperties
      }
      role="dialog"
      aria-label={title}
      onPointerDown={() => raise(id)}
    >
      <header
        className="notes-float-bar"
        onPointerDown={begin("move")}
        onPointerMove={move}
        onPointerUp={end}
        onPointerCancel={end}
        onDoubleClick={() => toggleMin(id)}
      >
        <strong title={title}>{title}</strong>
        <button
          type="button"
          className="icon-button"
          title={t("Open in notes")}
          aria-label={t("Open in notes")}
          onClick={() => {
            close(id);
            navigate(`/notes/${id}`);
          }}
        >
          <Maximize2 size={13} />
        </button>
        <button
          type="button"
          className="icon-button"
          title={min ? t("Restore window") : t("Minimize")}
          aria-label={min ? t("Restore window") : t("Minimize")}
          onClick={() => toggleMin(id)}
        >
          <Minus size={14} />
        </button>
        <button
          type="button"
          className="icon-button"
          title={t("Close")}
          aria-label={t("Close")}
          onClick={() => close(id)}
        >
          <X size={14} />
        </button>
      </header>
      {!min && (
        <div className="notes-float-body">
          <NoteEditor
            id={id}
            backTo="/notes"
            floating
            onClosed={() => close(id)}
          />
        </div>
      )}
      {!min && (
        <span
          className="notes-float-resize"
          aria-hidden
          onPointerDown={begin("size")}
          onPointerMove={move}
          onPointerUp={end}
          onPointerCancel={end}
        />
      )}
    </section>
  );
}

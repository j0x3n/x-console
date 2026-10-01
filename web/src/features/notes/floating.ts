import { create } from "zustand";

/*
 * 笔记浮窗（B72）。点编辑区右上角的按钮，笔记在右下角的浮窗里打开，
 * 换页面也还在（挂在 GlobalPanels 里）。最多 3 个，再开时关掉最早的。
 * 位置和大小记在 localStorage。
 */

export const MAX_FLOATING = 3;
const KEY = "xc.notes.floating";

export interface FloatBox {
  x: number;
  y: number;
  w: number;
  h: number;
}

interface FloatingState {
  ids: number[];
  minimized: number[];
  boxes: Record<number, FloatBox>;
  open: (id: number) => void;
  close: (id: number) => void;
  toggleMin: (id: number) => void;
  /** 点哪个浮窗，哪个放到最上面 */
  raise: (id: number) => void;
  setBox: (id: number, box: FloatBox) => void;
}

function read(): Pick<FloatingState, "ids" | "boxes"> {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? "{}");
    return {
      ids: Array.isArray(v.ids)
        ? v.ids.filter((x: unknown) => typeof x === "number")
        : [],
      boxes: v.boxes && typeof v.boxes === "object" ? v.boxes : {},
    };
  } catch {
    return { ids: [], boxes: {} };
  }
}

function write(s: Pick<FloatingState, "ids" | "boxes">) {
  try {
    localStorage.setItem(KEY, JSON.stringify({ ids: s.ids, boxes: s.boxes }));
  } catch {
    /* 记不住就算了 */
  }
}

export const useFloatingNotes = create<FloatingState>()((set, get) => ({
  ...read(),
  minimized: [],
  open: (id) => {
    const ids = [...get().ids.filter((x) => x !== id), id].slice(-MAX_FLOATING);
    set({ ids, minimized: get().minimized.filter((x) => x !== id) });
    write(get());
  },
  close: (id) => {
    set({
      ids: get().ids.filter((x) => x !== id),
      minimized: get().minimized.filter((x) => x !== id),
    });
    write(get());
  },
  toggleMin: (id) =>
    set({
      minimized: get().minimized.includes(id)
        ? get().minimized.filter((x) => x !== id)
        : [...get().minimized, id],
    }),
  raise: (id) => {
    const ids = get().ids;
    if (ids[ids.length - 1] === id) return;
    set({ ids: [...ids.filter((x) => x !== id), id] });
  },
  setBox: (id, box) => {
    set({ boxes: { ...get().boxes, [id]: box } });
    write(get());
  },
}));

/** 新浮窗的默认位置：右下角，每多一个往左上错开一点。 */
export function defaultBox(index: number, vw: number, vh: number): FloatBox {
  const w = Math.min(460, vw - 32);
  const h = Math.min(560, vh - 120);
  return {
    x: Math.max(16, vw - w - 96 - index * 28),
    y: Math.max(16, vh - h - 24 - index * 28),
    w,
    h,
  };
}

/** 拖动或改大小以后，浮窗不能跑出屏幕。 */
export function clampBox(b: FloatBox, vw: number, vh: number): FloatBox {
  const w = Math.max(300, Math.min(b.w, vw - 16));
  const h = Math.max(220, Math.min(b.h, vh - 16));
  return {
    w,
    h,
    x: Math.min(Math.max(8, b.x), vw - Math.min(w, 120)),
    y: Math.min(Math.max(8, b.y), vh - 44),
  };
}

import { create } from "zustand";

/*
 * 助手浮窗的界面状态。当前对话 id 记在 localStorage，刷新后还是同一个对话。
 * streaming 是正在生成的文字，按对话分开存，收到 ai.message_done 后清空。
 */
const CONV_KEY = "xc.ai.conversation";

function readConversation(): number | null {
  try {
    const v = Number(localStorage.getItem(CONV_KEY));
    return v > 0 ? v : null;
  } catch {
    return null;
  }
}

interface AssistantState {
  open: boolean;
  expanded: boolean;
  /** 浮窗离右下角的偏移，拖动后改变。 */
  offset: { x: number; y: number };
  conversationId: number | null;
  streaming: Record<number, string>;
  errors: Record<number, string>;
  setOpen: (open: boolean) => void;
  toggle: () => void;
  setExpanded: (expanded: boolean) => void;
  setOffset: (offset: { x: number; y: number }) => void;
  setConversation: (id: number | null) => void;
  appendDelta: (id: number, text: string) => void;
  clearStreaming: (id: number) => void;
  setError: (id: number, message: string) => void;
}

export const useAssistant = create<AssistantState>()((set, get) => ({
  open: false,
  expanded: false,
  offset: { x: 0, y: 0 },
  conversationId: readConversation(),
  streaming: {},
  errors: {},
  setOpen: (open) => set({ open }),
  toggle: () => set({ open: !get().open }),
  setExpanded: (expanded) => set({ expanded }),
  setOffset: (offset) => set({ offset }),
  setConversation: (id) => {
    try {
      if (id) localStorage.setItem(CONV_KEY, String(id));
      else localStorage.removeItem(CONV_KEY);
    } catch {
      /* 记不住就算了 */
    }
    set({ conversationId: id });
  },
  appendDelta: (id, text) => {
    const { streaming, errors } = get();
    const { [id]: _drop, ...rest } = errors;
    set({
      streaming: { ...streaming, [id]: (streaming[id] ?? "") + text },
      errors: rest,
    });
  },
  clearStreaming: (id) => {
    const { [id]: _drop, ...rest } = get().streaming;
    set({ streaming: rest });
  },
  setError: (id, message) => {
    const { [id]: _drop, ...rest } = get().streaming;
    set({ streaming: rest, errors: { ...get().errors, [id]: message } });
  },
}));

/** 拖动时把偏移限制在窗口里。 */
export function clampOffset(
  offset: { x: number; y: number },
  panel: { width: number; height: number },
  viewport: { width: number; height: number },
  margin = 20,
) {
  const maxX = Math.max(0, viewport.width - panel.width - margin * 2);
  const maxY = Math.max(0, viewport.height - panel.height - margin * 2);
  return {
    x: Math.min(Math.max(0, offset.x), maxX),
    y: Math.min(Math.max(0, offset.y), maxY),
  };
}

import { useEffect, useRef, useState } from "react";
import { create } from "zustand";
import { useT } from "../../contexts/LanguageContext";
import Dialog from "./Dialog";

/*
 * 高危操作的二次确认（B21）。删除、停止、重启、结束进程、吊销这类操作都先调它：
 *
 *   if (!(await confirmAction({ title: `删除容器 ${name}？`, description: "…" }))) return;
 *
 * 标题写清对象，说明写后果，不写“确定吗？”。批量操作写数量。
 * 需要更慎重时传 typeToConfirm，用户要输入这段文字才能点确认。
 */
export interface ConfirmOptions {
  title: string;
  description?: string;
  /** 确认按钮的文字，默认“删除”。写动作：“停止”“结束进程”“吊销”。 */
  confirmLabel?: string;
  /** 确认按钮用 danger 样式。默认 true。 */
  danger?: boolean;
  /** 要输入这段文字才能确认，比如“恢复”。 */
  typeToConfirm?: string;
}

interface Pending extends ConfirmOptions {
  resolve: (ok: boolean) => void;
}

interface ConfirmState {
  pending: Pending | null;
  hosts: number;
}

const useConfirmStore = create<ConfirmState>()(() => ({
  pending: null,
  hosts: 0,
}));

export function confirmAction(options: ConfirmOptions): Promise<boolean> {
  const state = useConfirmStore.getState();
  // 没有挂载弹窗时（单独渲染页面的测试）用浏览器自带的确认框。
  if (state.hosts === 0) {
    try {
      return Promise.resolve(!!window.confirm(options.title));
    } catch {
      return Promise.resolve(false);
    }
  }
  state.pending?.resolve(false);
  return new Promise((resolve) =>
    useConfirmStore.setState({ pending: { ...options, resolve } }),
  );
}

/** 组件里用的写法，等同于 confirmAction。 */
export function useConfirm() {
  return confirmAction;
}

/** 放在 Layout 里一次。 */
export function ConfirmHost() {
  const t = useT();
  const pending = useConfirmStore((s) => s.pending);
  const [typed, setTyped] = useState("");
  const cancelRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    useConfirmStore.setState((s) => ({ hosts: s.hosts + 1 }));
    return () => useConfirmStore.setState((s) => ({ hosts: s.hosts - 1 }));
  }, []);
  useEffect(() => {
    setTyped("");
    if (pending && !pending.typeToConfirm) cancelRef.current?.focus();
  }, [pending]);
  if (!pending) return null;
  const close = (ok: boolean) => {
    pending.resolve(ok);
    useConfirmStore.setState({ pending: null });
  };
  const blocked =
    !!pending.typeToConfirm && typed.trim() !== pending.typeToConfirm;
  return (
    <Dialog
      open
      onClose={() => close(false)}
      title={pending.title}
      description={pending.description}
      footer={
        <>
          <button
            ref={cancelRef}
            className="xc-btn"
            onClick={() => close(false)}
          >
            {t("Cancel")}
          </button>
          <button
            className={`xc-btn ${pending.danger === false ? "primary" : "danger solid"}`}
            disabled={blocked}
            onClick={() => close(true)}
          >
            {pending.confirmLabel ?? t("Delete")}
          </button>
        </>
      }
    >
      {pending.typeToConfirm && (
        <label className="xc-field">
          <span>
            {t("Type this to confirm:")}{" "}
            <strong>{pending.typeToConfirm}</strong>
          </span>
          <input
            className="xc-input"
            autoFocus
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !blocked) close(true);
            }}
          />
        </label>
      )}
    </Dialog>
  );
}

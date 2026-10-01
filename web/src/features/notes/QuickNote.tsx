import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { StickyNote } from "lucide-react";
import { create } from "zustand";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useCreateNote } from "./api";

/* 快速记录：页头按钮打开一个小框，写完直接存成新笔记。命令面板也能打开它。 */

interface QuickNoteState {
  open: boolean;
  setOpen: (open: boolean) => void;
}

export const useQuickNote = create<QuickNoteState>()((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
}));

export default function QuickNote() {
  const t = useT();
  const navigate = useNavigate();
  const open = useQuickNote((s) => s.open);
  const setOpen = useQuickNote((s) => s.setOpen);
  const [text, setText] = useState("");
  const ref = useRef<HTMLDivElement>(null);
  const createNote = useCreateNote();

  useEffect(() => {
    if (!open) return;
    const onDown = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open, setOpen]);

  const save = () => {
    if (!text.trim()) return;
    createNote.mutate(
      { body: text, quick: true },
      {
        onSuccess: () => {
          setText("");
          setOpen(false);
          toast(t("Saved to notes"));
        },
      },
    );
  };

  return (
    <div className="notes-quick-wrap" ref={ref}>
      <button
        className="icon-button"
        aria-label={t("Quick note")}
        title={t("Quick note")}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <StickyNote size={16} />
      </button>
      {open && (
        <div className="notes-quick" role="dialog" aria-label={t("Quick note")}>
          <textarea
            className="xc-textarea"
            autoFocus
            rows={5}
            value={text}
            placeholder={t("Write it down. It is saved as a new note.")}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) save();
              if (e.key === "Escape") setOpen(false);
            }}
          />
          <div className="xc-row">
            <button
              className="xc-btn ghost small"
              onClick={() => {
                setOpen(false);
                navigate("/notes");
              }}
            >
              {t("All notes")}
            </button>
            <span className="xc-spacer" />
            <span className="xc-muted notes-hint">⌘/Ctrl + Enter</span>
            <button
              className="xc-btn primary small"
              disabled={!text.trim() || createNote.isPending}
              onClick={save}
            >
              {t("Save")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

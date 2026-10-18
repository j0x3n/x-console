import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Archive, Mail, MailOpen, Trash2 } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import type { MailBatchAction, MailSummary } from "../api";

const EDGE = 8;

/**
 * 邮件列表的右键菜单（XC-66）：单独标已读或未读、归档、删除。
 * 样式用 MoreMenu 的 .xc-more-menu，位置跟着鼠标，放不下时往左上挪。
 */
export default function MailRowMenu({
  m,
  at,
  onAction,
  onClose,
}: {
  m: MailSummary;
  at: { x: number; y: number };
  onAction: (action: MailBatchAction) => void;
  onClose: () => void;
}) {
  const t = useT();
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);

  useLayoutEffect(() => {
    const r = ref.current?.getBoundingClientRect();
    if (!r) return;
    setPos({
      left: Math.max(EDGE, Math.min(at.x, window.innerWidth - EDGE - r.width)),
      top:
        at.y + r.height <= window.innerHeight - EDGE
          ? at.y
          : Math.max(EDGE, at.y - r.height),
    });
  }, [at.x, at.y]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    window.addEventListener("resize", onClose);
    window.addEventListener("scroll", onClose, true);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("scroll", onClose, true);
    };
  }, [onClose]);

  const items: {
    action: MailBatchAction;
    label: string;
    icon: ReactNode;
    danger?: boolean;
  }[] = [
    m.unread
      ? {
          action: "read",
          label: t("Mark as read"),
          icon: <MailOpen size={14} />,
        }
      : {
          action: "unread",
          label: t("Mark as unread"),
          icon: <Mail size={14} />,
        },
    { action: "archive", label: t("Archive"), icon: <Archive size={14} /> },
    {
      action: "delete",
      label: t("Delete"),
      icon: <Trash2 size={14} />,
      danger: true,
    },
  ];

  return createPortal(
    <>
      <div
        className="xc-more-backdrop"
        onClick={onClose}
        onContextMenu={(e) => {
          e.preventDefault();
          onClose();
        }}
      />
      <div
        ref={ref}
        className="xc-more-menu"
        role="menu"
        aria-label={m.subject || t("(No subject)")}
        style={pos ?? { top: at.y, left: at.x, visibility: "hidden" }}
      >
        {items.map((item) => (
          <button
            key={item.action}
            type="button"
            role="menuitem"
            className={item.danger ? "danger" : ""}
            onClick={() => {
              onClose();
              onAction(item.action);
            }}
          >
            {item.icon}
            {item.label}
          </button>
        ))}
      </div>
    </>,
    document.body,
  );
}

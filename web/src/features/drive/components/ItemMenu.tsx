import { useState, type ReactNode } from "react";
import {
  Download,
  Ellipsis,
  Eye,
  EyeOff,
  FolderInput,
  Link2,
  Pencil,
  RotateCcw,
  Trash2,
} from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import type { DriveItem } from "../api";

export type ItemAction =
  | "download"
  | "rename"
  | "move"
  | "copy-link"
  | "hide"
  | "unhide"
  | "trash"
  | "restore"
  | "delete-forever";

/** 条目在当前位置能做的操作。 */
export function itemActions(
  item: DriveItem,
  opts: { trash: boolean; vaultUnlocked: boolean },
): ItemAction[] {
  if (opts.trash) return ["restore", "delete-forever"];
  const out: ItemAction[] = [];
  if (!item.isDir) out.push("download");
  out.push("rename", "move");
  if (!item.isDir) out.push("copy-link");
  if (opts.vaultUnlocked) out.push(item.hidden ? "unhide" : "hide");
  out.push("trash");
  return out;
}

const LABELS: Record<ItemAction, string> = {
  download: "Download",
  rename: "Rename",
  move: "Move",
  "copy-link": "Copy link",
  hide: "Hide item",
  unhide: "Unhide item",
  trash: "Move to trash",
  restore: "Restore",
  "delete-forever": "Delete forever",
};

const ICONS: Record<ItemAction, ReactNode> = {
  download: <Download size={14} />,
  rename: <Pencil size={14} />,
  move: <FolderInput size={14} />,
  "copy-link": <Link2 size={14} />,
  hide: <EyeOff size={14} />,
  unhide: <Eye size={14} />,
  trash: <Trash2 size={14} />,
  restore: <RotateCcw size={14} />,
  "delete-forever": <Trash2 size={14} />,
};

/*
 * “更多”菜单。桌面上是按钮下方的下拉，手机上是底部弹出的菜单（见 drive.css）。
 */
export default function ItemMenu({
  item,
  actions,
  onAction,
}: {
  item: DriveItem;
  actions: ItemAction[];
  onAction: (action: ItemAction, item: DriveItem) => void;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <div className="drive-menu-wrap">
      <button
        type="button"
        className="xc-btn ghost small drive-more"
        aria-label={`${t("More")}: ${item.name}`}
        aria-expanded={open}
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
      >
        <Ellipsis size={15} />
      </button>
      {open && (
        <>
          <div
            className="drive-menu-backdrop"
            onClick={(e) => {
              e.stopPropagation();
              setOpen(false);
            }}
          />
          <div
            className="drive-menu"
            role="menu"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="drive-menu-title">{item.name}</div>
            {actions.map((action) => (
              <button
                key={action}
                role="menuitem"
                className={
                  action === "trash" || action === "delete-forever"
                    ? "danger"
                    : ""
                }
                onClick={() => {
                  setOpen(false);
                  onAction(action, item);
                }}
              >
                {ICONS[action]}
                {t(LABELS[action])}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

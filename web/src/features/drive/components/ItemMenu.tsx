import type { ReactNode } from "react";
import {
  Download,
  Eye,
  EyeOff,
  FolderInput,
  Link2,
  Pencil,
  RotateCcw,
  Trash2,
} from "lucide-react";
import MoreMenu from "../../../components/ui/MoreMenu";
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

/** 条目的“更多”菜单，用公共的 MoreMenu。 */
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
  return (
    <MoreMenu
      title={item.name}
      label={`${t("More")}: ${item.name}`}
      items={actions.map((action) => ({
        key: action,
        label: t(LABELS[action]),
        icon: ICONS[action],
        danger: action === "trash" || action === "delete-forever",
        onSelect: () => onAction(action, item),
      }))}
    />
  );
}

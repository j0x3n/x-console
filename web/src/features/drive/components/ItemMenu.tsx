import type { ReactNode } from "react";
import {
  Copy,
  FileArchive,
  FolderOpen,
  History,
  Share2,
  Download,
  Eye,
  FileText,
  SquarePen,
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
import { canEdit } from "../viewer/kind";
import { canExtract } from "../logic";

export type ItemAction =
  | "preview"
  | "edit"
  | "download"
  | "rename"
  | "move"
  | "copy"
  | "share"
  | "compress"
  | "extract"
  | "versions"
  | "copy-link"
  | "hide"
  | "unhide"
  | "trash"
  | "restore"
  | "delete-forever";

/** 条目在当前位置能做的操作。 */
export function itemActions(
  item: DriveItem,
  opts: {
    trash: boolean;
    vaultUnlocked: boolean;
    /** 在隐藏空间里。隐藏的东西不能分享。 */
    hiddenView?: boolean;
    /** 服务端有 B31 的打包下载，文件夹也能下载。 */
    batchLive?: boolean;
  },
): ItemAction[] {
  if (opts.trash) return ["restore", "delete-forever"];
  const out: ItemAction[] = [];
  if (!item.isDir) out.push("preview");
  if (canEdit(item)) out.push("edit");
  if (!item.isDir || opts.batchLive) out.push("download");
  if (!item.hidden && !opts.hiddenView) out.push("share");
  out.push("rename", "move", "copy", "compress");
  if (canExtract(item)) out.push("extract");
  if (!item.isDir) out.push("versions", "copy-link");
  if (opts.vaultUnlocked) out.push(item.hidden ? "unhide" : "hide");
  out.push("trash");
  return out;
}

const LABELS: Record<ItemAction, string> = {
  preview: "Preview",
  edit: "Edit",
  download: "Download",
  rename: "Rename",
  move: "Move",
  copy: "Copy to",
  share: "Share",
  compress: "Compress",
  extract: "Extract",
  versions: "Version history",
  "copy-link": "Copy link",
  hide: "Hide item",
  unhide: "Unhide item",
  trash: "Move to trash",
  restore: "Restore",
  "delete-forever": "Delete forever",
};

const ICONS: Record<ItemAction, ReactNode> = {
  preview: <FileText size={14} />,
  edit: <SquarePen size={14} />,
  download: <Download size={14} />,
  rename: <Pencil size={14} />,
  move: <FolderInput size={14} />,
  copy: <Copy size={14} />,
  share: <Share2 size={14} />,
  compress: <FileArchive size={14} />,
  extract: <FolderOpen size={14} />,
  versions: <History size={14} />,
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

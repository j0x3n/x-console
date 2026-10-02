import {
  AlertTriangle,
  CalendarClock,
  Code,
  EyeOff,
  Film,
  Highlighter,
  Info,
  Lightbulb,
  ListOrdered,
  Minus,
  OctagonAlert,
  Paperclip,
  Plus,
  Strikethrough,
  Table,
} from "lucide-react";
import MoreMenu, { type MoreMenuItem } from "../ui/MoreMenu";
import { useT } from "../../contexts/LanguageContext";
import { insertBlock, prefixLines, wrapSelection, type Edit } from "./edit";

type Apply = (fn: (text: string, start: number, end: number) => Edit) => void;

/** 插入 3 列 2 行的表格。 */
export const TABLE_TEMPLATE =
  "| 列一 | 列二 | 列三 |\n| --- | --- | --- |\n|  |  |  |\n|  |  |  |";

/** 现在的日期时间，比如 2026-10-02 14:05。 */
export function nowStamp(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

/**
 * 编辑工具条的“插入”下拉（B74）：不常用的格式放在这里。
 * 笔记编辑器和公共的 MarkdownEditor 都用它。
 * onMedia、onAttach 传了才有“视频或音频”“附件”两项（要能上传）。
 */
export default function InsertMenu({
  apply,
  onMedia,
  onAttach,
}: {
  apply: Apply;
  onMedia?: () => void;
  onAttach?: () => void;
}) {
  const t = useT();
  const block = (text: string) => () =>
    apply((x, s, e) => insertBlock(x, s, e, text));
  const container = (kind: string, title: string) => () =>
    apply((x, s, e) =>
      insertBlock(
        x,
        s,
        e,
        `:::${kind}${title ? ` ${title}` : ""}\n${x.slice(s, e) || t("Write here")}\n:::`,
      ),
    );
  const items: MoreMenuItem[] = [
    {
      key: "ol",
      label: t("Numbered list"),
      icon: <ListOrdered size={14} />,
      onSelect: () => apply((x, s, e) => prefixLines(x, s, e, "", true)),
    },
    {
      key: "del",
      label: t("Strikethrough"),
      icon: <Strikethrough size={14} />,
      onSelect: () =>
        apply((x, s, e) => wrapSelection(x, s, e, "~~", "~~", t("text"))),
    },
    {
      key: "mark",
      label: t("Highlight"),
      icon: <Highlighter size={14} />,
      onSelect: () =>
        apply((x, s, e) => wrapSelection(x, s, e, "==", "==", t("text"))),
    },
    {
      key: "inline-code",
      label: t("Inline code"),
      icon: <Code size={14} />,
      onSelect: () =>
        apply((x, s, e) => wrapSelection(x, s, e, "`", "`", "code")),
    },
    {
      key: "table",
      label: t("Table"),
      icon: <Table size={14} />,
      onSelect: block(TABLE_TEMPLATE),
    },
  ];
  if (onMedia)
    items.push({
      key: "media",
      label: t("Video or audio"),
      icon: <Film size={14} />,
      onSelect: onMedia,
    });
  items.push(
    {
      key: "hidden",
      label: t("Hidden block"),
      icon: <EyeOff size={14} />,
      onSelect: container("hidden", t("Hidden content")),
    },
    {
      key: "note",
      label: t("Callout note"),
      icon: <Info size={14} />,
      onSelect: container("note", ""),
    },
    {
      key: "tip",
      label: t("Callout tip"),
      icon: <Lightbulb size={14} />,
      onSelect: container("tip", ""),
    },
    {
      key: "warn",
      label: t("Callout warning"),
      icon: <AlertTriangle size={14} />,
      onSelect: container("warn", ""),
    },
    {
      key: "danger",
      label: t("Callout danger"),
      icon: <OctagonAlert size={14} />,
      onSelect: container("danger", ""),
    },
    {
      key: "hr",
      label: t("Divider"),
      icon: <Minus size={14} />,
      onSelect: block("---"),
    },
    {
      key: "now",
      label: t("Current date and time"),
      icon: <CalendarClock size={14} />,
      onSelect: () =>
        apply((x, s, e) => {
          const stamp = nowStamp();
          return {
            text: x.slice(0, s) + stamp + x.slice(e),
            start: s + stamp.length,
            end: s + stamp.length,
          };
        }),
    },
  );
  if (onAttach)
    items.push({
      key: "attach",
      label: t("Attach a file"),
      icon: <Paperclip size={14} />,
      onSelect: onAttach,
    });
  return (
    <MoreMenu
      className="xc-insert-menu"
      label={t("Insert")}
      title={t("Insert")}
      icon={<Plus size={15} />}
      items={items}
    />
  );
}

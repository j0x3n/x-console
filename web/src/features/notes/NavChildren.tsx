import { Pin } from "lucide-react";
import NavChildLinks from "../../components/layout/NavChildLinks";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useNotes } from "./api";
import { noteTitle } from "./logic";

/** 侧边栏“笔记”下面：置顶的在前，然后是最近改过的。 */
export default function NotesNavChildren({ onNavigate }: NavChildrenProps) {
  const notes = useNotes({
    q: "",
    tag: "",
    archived: false,
    pinned: false,
    hidden: false,
  });
  const items = notes.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <NavChildLinks
      links={items.map((n) => ({
        key: n.id,
        to: `/notes/${n.id}`,
        label: noteTitle(n.title, n.excerpt) || "无标题笔记",
        mark: n.pinned ? (
          <Pin size={11} className="nav-child-mark" />
        ) : undefined,
      }))}
      total={notes.hasNextPage ? items.length + 1 : items.length}
      allTo="/notes"
      loading={notes.isPending}
      error={notes.isError}
      empty="还没有笔记"
      onNavigate={onNavigate}
    />
  );
}

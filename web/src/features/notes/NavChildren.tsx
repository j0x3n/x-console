import { useLocation } from "react-router";
import NavChildLinks from "../../components/layout/NavChildLinks";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useTags } from "./api";
import { tagColor } from "./tagColor";

/** 侧边栏“笔记”下面：标签，前面是标签颜色，后面是笔记数。 */
export default function NotesNavChildren({ onNavigate }: NavChildrenProps) {
  const tags = useTags();
  const location = useLocation();
  const current = location.pathname.startsWith("/notes")
    ? new URLSearchParams(location.search).get("tag")
    : null;
  return (
    <NavChildLinks
      links={(tags.data ?? []).map((tc) => ({
        key: tc.tag,
        to: `/notes?tag=${encodeURIComponent(tc.tag)}`,
        label: tc.tag,
        hint: String(tc.count),
        active: current === tc.tag,
        mark: (
          <i
            className="nav-child-dot"
            style={{ background: tagColor(tc.tag, tc.color) }}
          />
        ),
      }))}
      allTo="/notes"
      loading={tags.isPending}
      error={tags.isError}
      empty="还没有标签"
      onNavigate={onNavigate}
    />
  );
}

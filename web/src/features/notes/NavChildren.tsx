import { useLocation } from "react-router";
import { Archive, EyeOff, Notebook, Pin, StickyNote } from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useVaultStatus } from "../vault/api";
import { useNoteCounts, useTags } from "./api";
import { tagColor } from "./tagColor";

/**
 * 左栏“笔记”的二级菜单（B102）：全部笔记、置顶、便签、已归档、隐藏（解锁后），下面是标签。
 * 地址参数和笔记页自己的分类栏一样。二级菜单显示时，笔记页不再显示那一栏（见 notes.css）。
 */
export default function NotesNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const location = useLocation();
  const params = new URLSearchParams(
    location.pathname.startsWith("/notes") ? location.search : "",
  );
  const vault = useVaultStatus();
  const unlocked = vault.data?.unlocked ?? false;
  const hidden = params.get("hidden") === "1" && unlocked;
  const counts = useNoteCounts();
  const tags = useTags(hidden);
  const tag = params.get("tag");
  const view = hidden
    ? "hidden"
    : tag
      ? "tag"
      : params.get("archived") === "1"
        ? "archived"
        : params.get("pinned") === "1"
          ? "pinned"
          : params.get("view") === "memos"
            ? "memos"
            : "all";
  const atList = location.pathname === "/notes";

  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/notes"
          icon={Notebook}
          label={t("All notes")}
          count={counts.data?.notes}
          active={atList && view === "all"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/notes?pinned=1"
          icon={Pin}
          label={t("Pinned notes")}
          count={counts.data?.pinned}
          active={view === "pinned"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/notes?view=memos"
          icon={StickyNote}
          label={t("Memos")}
          count={counts.data?.memos}
          active={view === "memos"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/notes?archived=1"
          icon={Archive}
          label={t("Archived")}
          count={counts.data?.archived}
          active={view === "archived"}
          onNavigate={onNavigate}
        />
        {unlocked && (
          <NavPanelLink
            to="/notes?hidden=1"
            icon={EyeOff}
            label={t("Hidden notes")}
            active={view === "hidden"}
            onNavigate={onNavigate}
          />
        )}
      </NavPanelGroup>
      <NavPanelGroup label={t("Tags")}>
        {tags.isPending ? (
          <div className="nav-children-note">{t("Loading")}…</div>
        ) : !tags.data?.length ? (
          <div className="nav-children-note">{t("No tags yet")}</div>
        ) : (
          tags.data.map((tc) => (
            <NavPanelLink
              key={tc.tag}
              to={`/notes?tag=${encodeURIComponent(tc.tag)}${hidden ? "&hidden=1" : ""}`}
              mark={
                <i
                  className="nav-child-dot"
                  style={{ background: tagColor(tc.tag, tc.color) }}
                />
              }
              label={tc.tag}
              count={tc.count}
              active={tag === tc.tag}
              onNavigate={onNavigate}
            />
          ))
        )}
      </NavPanelGroup>
    </NavPanelStack>
  );
}

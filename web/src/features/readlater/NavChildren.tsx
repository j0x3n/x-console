import { useSearchParams } from "react-router";
import { Bookmark, Check, Files, Tag } from "lucide-react";
import {
  NavPanelStack,
  NavPanelGroup,
  NavPanelLink,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useReadList } from "./api";
export default function NavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const [params] = useSearchParams();
  const view = params.get("view") ?? "unread";
  const tag = params.get("tag") ?? "";
  const list = useReadList("all", "", "");
  return (
    <NavPanelStack>
      <NavPanelGroup>
        {(
          [
            ["unread", "Unread", Bookmark],
            ["read", "Read", Check],
            ["all", "All", Files],
          ] as const
        ).map(([key, label, icon]) => (
          <NavPanelLink
            key={key}
            to={key === "unread" ? "/readlater" : `/readlater?view=${key}`}
            icon={icon}
            label={t(label)}
            count={list.data?.counts[key]}
            active={view === key && !tag}
            onNavigate={onNavigate}
          />
        ))}
      </NavPanelGroup>
      {!!list.data?.tags.length && (
        <NavPanelGroup label={t("Tags")}>
          {list.data.tags.map((item) => (
            <NavPanelLink
              key={item.tag}
              to={`/readlater?view=${view}&tag=${encodeURIComponent(item.tag)}`}
              icon={Tag}
              label={item.tag}
              count={item.count}
              active={tag === item.tag}
              onNavigate={onNavigate}
            />
          ))}
        </NavPanelGroup>
      )}
    </NavPanelStack>
  );
}

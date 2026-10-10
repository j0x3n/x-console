import { useLocation } from "react-router";
import { AlarmClock, Files, TriangleAlert } from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useDocuments } from "./api";
import { KINDS, KIND_ICONS, KIND_LABELS } from "./format";

/**
 * 左栏“证件档案”的二级菜单：全部、已过期、快到期，下面按类型分。
 * 页面里不再放类型标签，类型和视图都在这里选（地址里的 view 和 kind）。
 */
export default function DocumentsNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const location = useLocation();
  const list = useDocuments(false);
  const items = list.data?.items ?? [];
  const summary = list.data?.summary;
  const params = new URLSearchParams(location.search);
  const view = params.get("view") ?? "";
  const kind = params.get("kind") ?? "";
  const here = location.pathname === "/documents";
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/documents"
          icon={Files}
          label={t("All documents")}
          count={summary?.total || null}
          active={here && !view && !kind}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/documents?view=expired"
          icon={TriangleAlert}
          label={t("Expired")}
          count={summary?.expired || null}
          danger
          active={here && view === "expired"}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/documents?view=soon"
          icon={AlarmClock}
          label={t("Expiring soon")}
          count={summary?.soon || null}
          active={here && view === "soon"}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup label={t("Types")}>
        {KINDS.map((k) => (
          <NavPanelLink
            key={k}
            to={`/documents?kind=${k}`}
            icon={KIND_ICONS[k]}
            label={t(KIND_LABELS[k])}
            count={items.filter((d) => d.kind === k).length || null}
            active={here && kind === k}
            onNavigate={onNavigate}
          />
        ))}
      </NavPanelGroup>
    </NavPanelStack>
  );
}

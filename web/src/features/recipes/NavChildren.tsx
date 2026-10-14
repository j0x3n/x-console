import { useLocation } from "react-router";
import { ChefHat, Globe, UtensilsCrossed } from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useCandidates } from "./api";

/** 菜谱的二级菜单（B210）：全部菜谱、饮食记录。 */
export default function NavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const { pathname } = useLocation();
  const meals = pathname.startsWith("/recipes/meals");
  const discover = pathname.startsWith("/recipes/discover");
  const candidates = useCandidates();
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/recipes"
          icon={ChefHat}
          label={t("All recipes")}
          active={!meals && !discover}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/recipes/meals"
          icon={UtensilsCrossed}
          label={t("Meal log")}
          active={meals}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/recipes/discover"
          icon={Globe}
          label={t("Find new recipes")}
          active={discover}
          count={candidates.data?.items.length || undefined}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}

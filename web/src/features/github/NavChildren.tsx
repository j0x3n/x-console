import { useLocation } from "react-router";
import { FolderGit2 } from "lucide-react";
import NavChildLinks from "../../components/layout/NavChildLinks";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useGitHubStatus } from "./api";
import { runOutcome } from "./logic";
import { useRepoList } from "./useRepoList";

/**
 * 左栏“仓库”的二级菜单（B70、B102）：全部仓库，下面是关注的仓库，
 * CI 失败的在前，前面是 CI 状态点。
 */
export default function ReposNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const status = useGitHubStatus();
  const configured = status.data?.configured === true;
  const list = useRepoList(configured);
  const location = useLocation();
  const current = new URLSearchParams(location.search).get("repo");
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/github"
          icon={FolderGit2}
          label={t("All repositories")}
          count={configured ? list.rows.length : null}
          active={location.pathname === "/github" && !current}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      <NavPanelGroup label={t("Watched repositories")}>
        <NavChildLinks
          links={list.rows.map((r) => {
            const tone = r.ci ? runOutcome(r.ci).tone : "";
            return {
              key: r.key,
              to: `/github?repo=${encodeURIComponent(r.key)}`,
              label: r.repo.split("/").pop() ?? r.repo,
              mark: (
                <i
                  className={`xc-dot ${tone === "ok" ? "ok" : tone === "danger" ? "danger" : tone === "warn" ? "warn" : ""}`}
                />
              ),
              hint: r.openPulls ? `PR ${r.openPulls}` : undefined,
              active: location.pathname === "/github" && current === r.key,
            };
          })}
          allTo="/github"
          loading={status.isPending || (configured && list.isPending)}
          error={list.isError}
          empty={configured ? "还没有关注的仓库" : "还没有连接 Git 账号"}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}

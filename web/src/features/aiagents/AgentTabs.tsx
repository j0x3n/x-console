import { Link } from "react-router";
import { useT } from "../../contexts/LanguageContext";
import "./i18n";

/**
 * 仓库页 Toolbar 的 start。原来是 Agent 管理页的三个页签（B47），B103 起切换在左栏二级菜单里，
 * 这里只留“Git 账号在设置里管理”的链接（B62）。
 */
export default function AgentTabs() {
  const t = useT();
  return (
    <Link to="/settings/git" className="aiagent-tabs-link">
      {t("Git accounts are managed in settings")}
    </Link>
  );
}

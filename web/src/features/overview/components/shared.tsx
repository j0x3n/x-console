import type { ReactNode } from "react";
import { Link } from "react-router";
import { ArrowRight } from "lucide-react";
import { ApiError, errorMessage } from "../../../api/client";
import { Spinner } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";

/** 卡片标题右边的“查看全部”链接。 */
export function MoreLink({ to, label }: { to: string; label?: string }) {
  const t = useT();
  return (
    <Link className="today-more" to={to}>
      {label ?? t("View all")} <ArrowRight size={13} />
    </Link>
  );
}

/** 集成没配置：接口返回 412 或 integration_not_configured。 */
export function isSetupNeeded(error: unknown) {
  return (
    error instanceof ApiError &&
    (error.status === 412 || error.code === "integration_not_configured")
  );
}

/**
 * 卡片里的查询状态：加载中、出错、没配置。都不是时返回 null，由卡片画内容。
 * setupTo 是“去设置”要跳到的设置页。
 */
export function QueryState({
  query,
  setupTo,
  setupHint,
}: {
  query: {
    isPending: boolean;
    isError: boolean;
    error: unknown;
    refetch: () => unknown;
  };
  setupTo?: string;
  setupHint?: string;
}) {
  const t = useT();
  if (query.isPending) return <Pending />;
  if (!query.isError) return null;
  if (setupTo && isSetupNeeded(query.error))
    return (
      <div className="today-note">
        <span>{setupHint ?? t("Not configured")}</span>
        <Link className="xc-btn small" to={setupTo}>
          {t("Go to settings")}
        </Link>
      </div>
    );
  return (
    <div className="today-note is-error">
      <span>{errorMessage(query.error)}</span>
      <button className="xc-btn small ghost" onClick={() => query.refetch()}>
        {t("Retry")}
      </button>
    </div>
  );
}

export function Pending() {
  return (
    <div className="today-note">
      <Spinner />
    </div>
  );
}

/** 卡片里的空状态，一句话加一个可选操作。 */
export function Empty({
  children,
  action,
}: {
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="today-note">
      <span>{children}</span>
      {action}
    </div>
  );
}

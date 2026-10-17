import { Link } from "react-router";
import { AlertTriangle, Newspaper, PlugZap, Unplug } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { feedsErrorCode } from "./api";

const SETTINGS = "?settings=feeds";

/** 还没配置：给“去设置”。 */
export function NotConfigured() {
  const t = useT();
  return (
    <div className="xc-empty">
      <Newspaper size={28} />
      <strong>{t("Feeds are not set up")}</strong>
      <span>
        {t("Choose the machine that runs ai-news and x-assistant in Settings.")}
      </span>
      <Link className="xc-btn primary small" to={SETTINGS}>
        {t("Go to settings")}
      </Link>
    </div>
  );
}

/** 读不到数据时按错误码说清楚原因。后端的提示已经是中文，直接显示。 */
export function FeedsError({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry: () => void;
}) {
  const t = useT();
  const code = feedsErrorCode(error);
  if (code === "not_configured") return <NotConfigured />;
  const title =
    code === "agent_offline"
      ? t("The machine is offline")
      : code === "feeds_unauthorized"
        ? t("The feeds service rejected the token")
        : code === "feeds_unreachable"
          ? t("The feeds service is not answering")
          : t("Something went wrong");
  const Icon =
    code === "agent_offline"
      ? Unplug
      : code === "feeds_unauthorized"
        ? PlugZap
        : AlertTriangle;
  return (
    <div className="xc-empty" role="alert">
      <Icon size={28} />
      <strong>{title}</strong>
      <span>{errorMessage(error)}</span>
      <span className="feeds-state-actions">
        <button className="xc-btn small" onClick={onRetry}>
          {t("Retry")}
        </button>
        {code === "feeds_unauthorized" && (
          <Link className="xc-btn small" to={SETTINGS}>
            {t("Go to settings")}
          </Link>
        )}
      </span>
    </div>
  );
}

import type { ReactNode } from "react";
import { AlertTriangle, Construction, Inbox } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";

export function Spinner() {
  return <span className="xc-spinner" role="status" aria-label="loading" />;
}

export function Loading() {
  return (
    <div className="xc-empty">
      <Spinner />
    </div>
  );
}

export function EmptyState({
  title,
  children,
  icon,
}: {
  title: string;
  children?: ReactNode;
  icon?: ReactNode;
}) {
  return (
    <div className="xc-empty">
      {icon ?? <Inbox size={28} />}
      <strong>{title}</strong>
      {children}
    </div>
  );
}

export function ErrorState({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  const t = useT();
  return (
    <div className="xc-empty">
      <AlertTriangle size={28} />
      <strong>{t("Something went wrong")}</strong>
      <span>{errorMessage(error)}</span>
      {onRetry && (
        <button className="xc-btn small" onClick={onRetry}>
          {t("Retry")}
        </button>
      )}
    </div>
  );
}

/**
 * 后端还没做的功能：接口返回 404 或 501 时整页显示这个，不要显示成“出错了”。
 * name 写功能名，比如“云盘”。
 */
export function NotLive({ name, icon }: { name: string; icon?: ReactNode }) {
  return (
    <div className="xc-empty">
      {icon ?? <Construction size={28} />}
      <strong>{name}还没上线</strong>
      <span>界面已经做好，服务端还在开发。</span>
    </div>
  );
}

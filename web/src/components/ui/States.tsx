import type { ReactNode } from "react";
import { AlertTriangle, Inbox } from "lucide-react";
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

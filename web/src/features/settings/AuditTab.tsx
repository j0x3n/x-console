import { useInfiniteQuery } from "@tanstack/react-query";
import { coreApi, coreKeys } from "../../api/core";
import { unwrap } from "../../api/client";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";

export default function AuditTab() {
  const t = useT();
  const language = useLanguage();
  const audit = useInfiniteQuery({
    queryKey: coreKeys.audit,
    queryFn: ({ pageParam }) =>
      unwrap(
        coreApi.GET("/audit", {
          params: { query: { limit: 50, cursor: pageParam || undefined } },
        }),
      ),
    initialPageParam: "",
    getNextPageParam: (last) => last.nextCursor,
  });
  if (audit.isPending) return <Loading />;
  if (audit.isError)
    return <ErrorState error={audit.error} onRetry={() => audit.refetch()} />;
  const rows = audit.data.pages.flatMap((p) => p.items);
  return (
    <div className="xc-card">
      <div className="xc-table-wrap">
        <table className="xc-table">
          <thead>
            <tr>
              <th>{t("Time")}</th>
              <th>{t("Actor")}</th>
              <th>{t("Action")}</th>
              <th>{t("Target")}</th>
              <th>{t("Result")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.id}>
                <td title={row.at}>{relativeTime(row.at, language)}</td>
                <td>{row.actor}</td>
                <td className="xc-mono">{row.action}</td>
                <td>{row.target}</td>
                <td>
                  <span
                    className={`xc-badge ${row.result === "ok" ? "ok" : "danger"}`}
                  >
                    {row.result}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {audit.hasNextPage && (
        <div style={{ textAlign: "center", marginTop: 12 }}>
          <button
            className="xc-btn small"
            disabled={audit.isFetchingNextPage}
            onClick={() => audit.fetchNextPage()}
          >
            {t("Load more")}
          </button>
        </div>
      )}
    </div>
  );
}

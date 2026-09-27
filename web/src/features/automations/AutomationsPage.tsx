import { Link } from "react-router";
import { Play, Plus, ShieldAlert, Workflow } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import Switch from "../../components/ui/Switch";
import { StatCard, StatStrip } from "../../components/ui/Stat";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { relativeTime } from "../../lib/time";
import {
  isNotLive,
  useAutomations,
  useCatalog,
  useRunAutomation,
  useToggleAutomation,
} from "./api";
import { RunBadge } from "./components/RunsList";
import { hasDangerous, triggerSummary } from "./logic";

export default function AutomationsPage() {
  const t = useT();
  const language = useLanguage();
  const list = useAutomations();
  const catalog = useCatalog();
  const toggle = useToggleAutomation();
  const run = useRunAutomation();

  const heading = (
    <PageHeading
      title={t("Automations")}
      subtitle="触发器满足条件时，按顺序执行动作。"
      aside={
        !(list.isError && isNotLive(list.error)) && (
          <Link className="xc-btn primary auto-link" to="/automations/new">
            <Plus size={15} /> {t("New rule")}
          </Link>
        )
      }
    />
  );

  if (list.isPending)
    return (
      <div className="xc-page">
        {heading}
        <Loading />
      </div>
    );
  if (list.isError)
    return (
      <div className="xc-page">
        {heading}
        {isNotLive(list.error) ? (
          <EmptyState title="自动化还没上线" icon={<Workflow size={28} />}>
            <span className="auto-muted">界面已经做好，服务端还在开发。</span>
          </EmptyState>
        ) : (
          <ErrorState error={list.error} onRetry={() => list.refetch()} />
        )}
      </div>
    );

  const rules = list.data;
  const enabled = rules.filter((r) => r.enabled).length;
  const failed = rules.filter((r) => r.lastRun?.status === "failed").length;
  const titles = new Map(
    (catalog.data?.actions ?? []).map((a) => [a.name, a.title]),
  );

  return (
    <div className="xc-page">
      {heading}
      <StatStrip label={t("Automations")}>
        <StatCard
          label={t("Rules")}
          value={rules.length}
          foot={`${enabled} ${t("enabled")}`}
        />
        <StatCard
          label={t("Last run failed")}
          value={failed}
          tone={failed > 0 ? "danger" : undefined}
          foot={failed > 0 ? "点进规则看运行记录" : "都正常"}
        />
      </StatStrip>

      {rules.length === 0 ? (
        <EmptyState title={t("No rules yet")} icon={<Workflow size={26} />}>
          <span className="auto-muted">
            比如：服务器 CPU 超过 90% 时发通知；每天早上 9 点让 AI
            总结昨天的告警。
          </span>
          <Link className="xc-btn small auto-link" to="/automations/new">
            <Plus size={14} /> {t("New rule")}
          </Link>
        </EmptyState>
      ) : (
        <div className="auto-list">
          {rules.map((r) => (
            <div key={r.id} className={`auto-card${r.enabled ? "" : " off"}`}>
              <Switch
                checked={r.enabled}
                label={`${r.name} ${t("enabled")}`}
                onChange={(enabled) => toggle.mutate({ id: r.id, enabled })}
              />
              <Link to={`/automations/${r.id}`} className="auto-card-main">
                <strong>{r.name}</strong>
                <span className="auto-muted">
                  {triggerSummary(r.trigger)} →{" "}
                  {r.actions
                    .map((s) => titles.get(s.action) ?? s.action)
                    .join("、")}
                </span>
              </Link>
              <div className="auto-card-side">
                {catalog.data &&
                  hasDangerous(r.actions, catalog.data.actions) &&
                  !r.authorized && (
                    <span
                      className="xc-badge warn"
                      title="有高危动作，保存时没有授权，不会执行"
                    >
                      <ShieldAlert size={12} /> {t("Needs authorization")}
                    </span>
                  )}
                {r.lastRun ? (
                  <>
                    <RunBadge run={r.lastRun} />
                    <small className="auto-muted">
                      {relativeTime(r.lastRun.startedAt, language)}
                    </small>
                  </>
                ) : (
                  <small className="auto-muted">{t("Never ran")}</small>
                )}
                <button
                  type="button"
                  className="xc-btn ghost small"
                  title={t("Run now")}
                  aria-label={`${t("Run now")} ${r.name}`}
                  disabled={run.isPending}
                  onClick={() => run.mutate(r.id)}
                >
                  <Play size={14} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

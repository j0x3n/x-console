import { useState } from "react";
import { Link } from "react-router";
import { FlaskConical, Plus } from "lucide-react";
import { isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { useExperiments } from "./api";
import ExperimentDialog from "./ExperimentDialog";
import { PHASE_LABELS, PHASE_TONE } from "./format";
import "./i18n";
import "./experiments.css";

/** 自我实验（B133）：定义改变和要看的数，用已有的数据比较前后。 */
export default function ExperimentsPage() {
  const t = useT();
  const list = useExperiments();
  const [creating, setCreating] = useState(false);

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = (
      <NotLive name={t("Experiments")} icon={<FlaskConical size={28} />} />
    );
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (list.data.items.length === 0)
    body = (
      <EmptyState
        title={t("No experiments yet")}
        icon={<FlaskConical size={28} />}
      >
        <span>
          {t(
            "Pick one change, such as no coffee after 2 pm, and the numbers to watch. A few weeks later this page compares the days before and after.",
          )}
        </span>
        <button className="xc-btn primary" onClick={() => setCreating(true)}>
          {t("New experiment")}
        </button>
      </EmptyState>
    );
  else
    body = (
      <ul className="experiments-list">
        {list.data.items.map((e) => (
          <li key={e.id}>
            <Link
              to={`/experiments/${e.id}`}
              className="xc-card experiments-item"
            >
              <div className="experiments-item-head">
                <strong>{e.title}</strong>
                <span className={`xc-badge ${PHASE_TONE[e.phase]}`}>
                  {t(PHASE_LABELS[e.phase])}
                </span>
              </div>
              {e.change && <p className="xc-muted">{e.change}</p>}
              <small className="xc-muted">
                {e.baselineStart} → {e.changeStart} → {e.endDate} ·{" "}
                {e.metrics.map((m) => m.label).join("、")}
              </small>
            </Link>
          </li>
        ))}
      </ul>
    );

  return (
    <div className="xc-page experiments-page">
      <PageHeading
        title={t("Experiments")}
        aside={
          <button
            className="xc-btn small primary"
            onClick={() => setCreating(true)}
          >
            <Plus size={14} /> {t("New experiment")}
          </button>
        }
      />
      {body}
      <ExperimentDialog open={creating} onClose={() => setCreating(false)} />
    </div>
  );
}

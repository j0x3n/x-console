import { useState } from "react";
import { Archive, GraduationCap, Pencil, Plus, Trash2 } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import MoreMenu from "../../components/ui/MoreMenu";
import PageHeading from "../../components/ui/PageHeading";
import { MiniBars, StatCard, StatStrip } from "../../components/ui/Stat";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useDeleteSkill, useSkills, useUpdateSkill, type Skill } from "./api";
import { formatMinutes, targetPercent, weekLabels } from "./format";
import SessionDialog from "./SessionDialog";
import SessionsPanel from "./SessionsPanel";
import SkillDialog from "./SkillDialog";
import "./i18n";
import "./skills.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 技能练习记录（B138）：每次练了多久、练了什么、哪里卡住，按周看投入。 */
export default function SkillsPage() {
  const t = useT();
  const [showArchived, setShowArchived] = useState(false);
  const list = useSkills(showArchived);
  const [dialog, setDialog] = useState<{ skill?: Skill } | null>(null);

  const items = list.data?.items ?? [];
  const active = items.filter((s) => !s.archived);
  const weekTotal = active.reduce((n, s) => n + s.weekMinutes, 0);
  const practiced = active.filter((s) => s.weekMinutes > 0).length;
  const withTarget = active.filter((s) => s.weeklyTarget);
  const met = withTarget.filter(
    (s) => s.weekMinutes >= (s.weeklyTarget ?? 0),
  ).length;

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Skills")} icon={<GraduationCap size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (items.length === 0)
    body = (
      <EmptyState
        title={t("Skill empty title")}
        icon={<GraduationCap size={28} />}
      >
        <span>{t("Skill empty body")}</span>
      </EmptyState>
    );
  else
    body = (
      <div className="sk-grid">
        {items.map((s) => (
          <SkillCard
            key={s.id}
            skill={s}
            today={list.data?.today}
            onEdit={() => setDialog({ skill: s })}
          />
        ))}
      </div>
    );

  return (
    <div className="xc-page sk-page">
      <PageHeading
        title={t("Skills")}
        aside={
          <button
            className="xc-btn small primary"
            onClick={() => setDialog({})}
          >
            <Plus size={14} /> {t("Skill new")}
          </button>
        }
      />
      <StatStrip label={t("Skills")}>
        <StatCard
          label={t("Skill week total")}
          value={formatMinutes(weekTotal)}
        />
        <StatCard label={t("Skill practiced count")} value={practiced} />
        {withTarget.length > 0 && (
          <StatCard
            label={t("Skill targets met")}
            value={`${met} / ${withTarget.length}`}
          />
        )}
      </StatStrip>
      <label className="xc-check">
        <input
          type="checkbox"
          checked={showArchived}
          onChange={(e) => setShowArchived(e.target.checked)}
        />
        <span>{t("Show archived")}</span>
      </label>
      {body}
      <p className="xc-muted sk-hint">{t("Skill evidence")}</p>
      <p className="xc-muted sk-hint">{t("Skill goal hint")}</p>
      <SkillDialog
        open={dialog !== null}
        onClose={() => setDialog(null)}
        skill={dialog?.skill}
      />
    </div>
  );
}

function SkillCard({
  skill,
  today,
  onEdit,
}: {
  skill: Skill;
  today?: string;
  onEdit: () => void;
}) {
  const t = useT();
  const update = useUpdateSkill();
  const remove = useDeleteSkill();
  const [logging, setLogging] = useState(false);
  const [open, setOpen] = useState(false);
  const percent = targetPercent(skill.weekMinutes, skill.weeklyTarget);

  const archive = () =>
    update.mutate(
      { id: skill.id, body: { archived: !skill.archived } },
      { onError: fail },
    );
  const del = async () => {
    const ok = await confirmAction({
      title: t("Skill delete title"),
      description: `${skill.name}：${t("Skill delete body")}`,
    });
    if (ok) remove.mutate(skill.id, { onError: fail });
  };

  return (
    <article
      className={`xc-card sk-card${skill.archived ? " archived" : ""}`}
      aria-label={skill.name}
    >
      <header className="sk-head">
        <h2 className="sk-name">{skill.name}</h2>
        {skill.archived && <span className="xc-badge">{t("Archived")}</span>}
        <MoreMenu
          title={skill.name}
          label={`${t("More")} ${skill.name}`}
          items={[
            {
              key: "edit",
              label: t("Edit"),
              icon: <Pencil size={14} />,
              onSelect: onEdit,
            },
            {
              key: "archive",
              label: t(skill.archived ? "Unarchive" : "Archive"),
              icon: <Archive size={14} />,
              onSelect: archive,
            },
            {
              key: "delete",
              label: t("Delete"),
              icon: <Trash2 size={14} />,
              danger: true,
              onSelect: del,
            },
          ]}
        />
      </header>
      <div className="sk-week">
        <strong>{formatMinutes(skill.weekMinutes)}</strong>
        <span className="xc-muted">
          {t("Skill this week")}
          {skill.weeklyTarget ? ` / ${formatMinutes(skill.weeklyTarget)}` : ""}
        </span>
      </div>
      {percent !== null && (
        <div
          className="sk-progress"
          role="progressbar"
          aria-valuenow={percent}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <span style={{ width: `${percent}%` }} />
        </div>
      )}
      <div className="sk-chart" aria-label={t("Skill last 8 weeks")}>
        <MiniBars
          values={skill.weeks}
          labels={weekLabels(skill.weeks.length)}
        />
      </div>
      <p className="sk-meta xc-muted">
        {t("Skill total")} {formatMinutes(skill.totalMinutes)} ·{" "}
        {t("Skill last practiced")}{" "}
        {skill.lastPracticedOn ?? t("Skill never practiced")}
      </p>
      <div className="sk-actions">
        <button
          className="xc-btn small primary"
          disabled={skill.archived}
          onClick={() => setLogging(true)}
        >
          {t("Skill log")}
        </button>
        <button
          className="xc-btn small"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          {t("Skill sessions")}
        </button>
      </div>
      {open && <SessionsPanel skill={skill} today={today} />}
      <SessionDialog
        open={logging}
        onClose={() => setLogging(false)}
        skillId={skill.id}
        skillName={skill.name}
        today={today}
      />
    </article>
  );
}

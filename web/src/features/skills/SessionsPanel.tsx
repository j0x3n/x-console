import { useState } from "react";
import { Pencil, Trash2 } from "lucide-react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useDeleteSession,
  useSkillSessions,
  type Skill,
  type SkillSession,
} from "./api";
import { formatMinutes } from "./format";
import SessionDialog from "./SessionDialog";

/** 一项技能的练习记录，可以只看卡住的，能改能删。 */
export default function SessionsPanel({
  skill,
  today,
}: {
  skill: Skill;
  today?: string;
}) {
  const t = useT();
  const [stuckOnly, setStuckOnly] = useState(false);
  const [editing, setEditing] = useState<SkillSession | null>(null);
  const list = useSkillSessions(skill.id, stuckOnly, true);
  const remove = useDeleteSession();

  const del = async (s: SkillSession) => {
    const ok = await confirmAction({
      title: t("Skill session delete title"),
      description: `${s.practicedOn} · ${formatMinutes(s.minutes)}`,
    });
    if (!ok) return;
    remove.mutate(s.id, {
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });
  };

  return (
    <div className="sk-sessions">
      <label className="xc-check">
        <input
          type="checkbox"
          checked={stuckOnly}
          onChange={(e) => setStuckOnly(e.target.checked)}
        />
        <span>{t("Skill stuck only")}</span>
      </label>
      {list.isPending && <Loading />}
      {list.data && list.data.items.length === 0 && (
        <p className="xc-muted">{t("Skill sessions empty")}</p>
      )}
      <ul className="sk-session-list">
        {list.data?.items.map((s) => (
          <li key={s.id}>
            <div className="sk-session-head">
              <span className="sk-day">{s.practicedOn}</span>
              <span>{formatMinutes(s.minutes)}</span>
              <span className="sk-session-actions">
                <button
                  className="xc-btn small ghost"
                  title={t("Edit")}
                  aria-label={`${t("Edit")} ${s.practicedOn}`}
                  onClick={() => setEditing(s)}
                >
                  <Pencil size={14} />
                </button>
                <button
                  className="xc-btn small ghost danger"
                  title={t("Delete")}
                  aria-label={`${t("Delete")} ${s.practicedOn}`}
                  onClick={() => del(s)}
                >
                  <Trash2 size={14} />
                </button>
              </span>
            </div>
            {s.content && <p className="sk-text">{s.content}</p>}
            {s.stuck && (
              <p className="sk-text sk-stuck">
                {t("Skill stuck")}：{s.stuck}
              </p>
            )}
          </li>
        ))}
      </ul>
      <SessionDialog
        open={editing !== null}
        onClose={() => setEditing(null)}
        skillId={skill.id}
        skillName={skill.name}
        today={today}
        session={editing ?? undefined}
      />
    </div>
  );
}

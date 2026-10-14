import { Link } from "react-router";
import { Pause, Play, Plus, X } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useDeleteTimer, useUpdateTimer, type CookTimer } from "./api";
import { isDue, remaining, sortTimers } from "./alarm";
import { clock } from "./durations";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 计时器列表（B209）：全局浮条展开后和做菜页都用它。 */
export default function TimerList({
  timers,
  now,
  showLinks,
  onNavigate,
}: {
  timers: CookTimer[];
  now: number;
  /** 显示菜名和第几步，能点到做菜页 */
  showLinks?: boolean;
  onNavigate?: () => void;
}) {
  return (
    <ul className="recipes-timers">
      {sortTimers(timers, now).map((timer) => (
        <TimerRow
          key={timer.id}
          timer={timer}
          now={now}
          showLink={showLinks}
          onNavigate={onNavigate}
        />
      ))}
    </ul>
  );
}

function TimerRow({
  timer,
  now,
  showLink,
  onNavigate,
}: {
  timer: CookTimer;
  now: number;
  showLink?: boolean;
  onNavigate?: () => void;
}) {
  const t = useT();
  const update = useUpdateTimer();
  const remove = useDeleteTimer();
  const due = isDue(timer, now);
  const left = remaining(timer, now);
  const where =
    timer.recipeTitle &&
    `${timer.recipeTitle}${timer.stepIndex != null ? ` · 第 ${timer.stepIndex + 1} 步` : ""}`;
  return (
    <li className={`recipes-timer${due ? " due" : ""}`}>
      <div className="recipes-timer-main">
        <strong>{timer.label}</strong>
        {where &&
          (showLink && timer.recipeId ? (
            <Link to={`/recipes/${timer.recipeId}/cook`} onClick={onNavigate}>
              {where}
            </Link>
          ) : (
            <small>{where}</small>
          ))}
      </div>
      <span className="recipes-timer-clock">
        {due ? t("Time is up") : clock(left)}
      </span>
      <div className="recipes-timer-actions">
        {!due && (
          <button
            className="xc-btn small ghost"
            title={timer.state === "paused" ? t("Continue timer") : t("Pause")}
            aria-label={
              timer.state === "paused" ? t("Continue timer") : t("Pause")
            }
            disabled={update.isPending}
            onClick={() =>
              update.mutate(
                {
                  id: timer.id,
                  body: {
                    action: timer.state === "paused" ? "resume" : "pause",
                  },
                },
                { onError: fail },
              )
            }
          >
            {timer.state === "paused" ? (
              <Play size={14} />
            ) : (
              <Pause size={14} />
            )}
          </button>
        )}
        <button
          className="xc-btn small ghost"
          title={t("Add 1 minute")}
          aria-label={t("Add 1 minute")}
          disabled={update.isPending}
          onClick={() =>
            update.mutate(
              { id: timer.id, body: { addSeconds: 60 } },
              { onError: fail },
            )
          }
        >
          <Plus size={14} />
          <span className="recipes-timer-plus">1′</span>
        </button>
        <button
          className={`xc-btn small${due ? " primary" : " ghost"}`}
          title={due ? t("Dismiss") : t("Cancel cooking timer")}
          aria-label={due ? t("Dismiss") : t("Cancel cooking timer")}
          disabled={remove.isPending}
          onClick={() => remove.mutate(timer.id, { onError: fail })}
        >
          <X size={14} />
          {due && <span>{t("Dismiss")}</span>}
        </button>
      </div>
    </li>
  );
}

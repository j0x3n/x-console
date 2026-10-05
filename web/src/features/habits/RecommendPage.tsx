import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Check, Dumbbell, Plus } from "lucide-react";
import { errorMessage } from "../../api/client";
import { Toolbar } from "../../components/ui/Toolbar";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useHabitList, useUpdateHabit, type Habit } from "./api";
import HabitDialog from "./HabitDialog";
import {
  useActivatePersonalHabits,
  usePersonalLibrary,
  usePersonalProfile,
} from "./personalApi";
import {
  EXTRA_RECOMMENDATIONS,
  FITNESS_PROGRAMS,
  TEMPLATE_RECOMMENDATIONS,
  libraryCategory,
  programHabitInput,
  recommendCategories,
  sameName,
  type RecommendCategory,
  type Recommendation,
} from "./recommend";
import type { HabitInput, HabitTemplate } from "./api";

/** 一张推荐卡片要的东西。三种来源（模板、个人计划、写死的）统一成这个。 */
interface Card {
  key: string;
  category: RecommendCategory;
  name: string;
  icon: string;
  summary: string;
  meta?: string;
  /** 已经建好的习惯，点了打开编辑 */
  habit?: Habit;
  /** 个人计划建的、后来归档了 */
  archived?: Habit;
  add: () => void;
  /** 健身方案：看动作 */
  program?: string;
}

/**
 * 推荐习惯（用户 2026-10-05 要求，原来“个人计划”的首页）。
 * 按分类列出能直接加的习惯，建过的显示“已添加”，点了打开编辑。
 * 个人计划的打卡模板一键加入；其他的打开新建弹窗并填好，可以先改提醒再保存。
 */
export default function RecommendPage() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const habits = useHabitList();
  const library = usePersonalLibrary();
  const profile = usePersonalProfile();
  const activate = useActivatePersonalHabits();
  const restore = useUpdateHabit();
  const [draft, setDraft] = useState<{
    input?: Omit<HabitInput, "activeHostIds">;
    template?: HabitTemplate;
  } | null>(null);
  const [editing, setEditing] = useState<Habit | null>(null);
  const cat = (params.get("cat") ?? "") as RecommendCategory | "";
  const setCat = (next: string) => {
    const p = new URLSearchParams(params);
    if (next) p.set("cat", next);
    else p.delete("cat");
    setParams(p, { replace: true });
  };

  if (habits.isPending) return <Loading />;
  if (habits.isError)
    return <ErrorState error={habits.error} onRetry={() => habits.refetch()} />;

  const all = habits.data;
  const active = all.filter((h) => !h.archived);
  const byName = (name: string) => active.find((h) => sameName(h.name, name));
  const usedTemplates = active
    .map((h) => h.template)
    .filter((x): x is HabitTemplate => !!x);

  const cards: Card[] = [
    ...TEMPLATE_RECOMMENDATIONS.map(
      (r): Card => ({
        ...base(r),
        meta: remindMeta(r.input, t),
        habit: active.find((h) => h.template === r.template),
        add: () => setDraft({ template: r.template }),
      }),
    ),
    // 个人计划的打卡模板。没连上服务端时这一部分不显示
    ...(library.data && profile.data
      ? library.data.habits.map((h): Card => {
          const id = profile.data.habitIds[h.id];
          const habit = all.find((x) => x.id === id);
          return {
            key: `lib-${h.id}`,
            category: libraryCategory(h),
            name: h.name,
            icon: libraryIcons[h.id] ?? (h.exerciseId ? "🧘" : "✅"),
            summary: h.tip,
            meta: `${t("Personal plan")} · ${h.group}`,
            habit: habit && !habit.archived ? habit : undefined,
            archived: habit?.archived ? habit : undefined,
            add: () =>
              activate.mutate([h.id], {
                onSuccess: () => toast(`${t("Template added")}：${h.name}`),
                onError: (e) =>
                  toast({ message: errorMessage(e), tone: "error" }),
              }),
          };
        })
      : []),
    ...EXTRA_RECOMMENDATIONS.map(
      (r): Card => ({
        ...base(r),
        meta: remindMeta(r.input, t),
        habit: byName(r.name),
        add: () => setDraft({ input: r.input }),
      }),
    ),
    ...FITNESS_PROGRAMS.map(
      (p): Card => ({
        key: `prog-${p.id}`,
        category: "body",
        name: p.habitName,
        icon: p.icon,
        summary: `${p.name}：${p.summary}`,
        meta: `${p.items.length} ${t("exercises")} · ${t("about")} ${p.minutes} ${t("minutes")} · ${p.remindAt}`,
        habit: byName(p.habitName),
        add: () => setDraft({ input: programHabitInput(p) }),
        program: p.id,
      }),
    ),
  ];

  const shown = cats(cards).filter((c) => !cat || c.id === cat);
  const added = cards.filter((c) => c.habit).length;
  const pending = activate.isPending || restore.isPending;

  return (
    <div className="xc-stack habits-rec">
      <Toolbar
        start={
          <nav
            className="xc-tabs habits-rec-tabs"
            aria-label={t("Recommendation categories")}
          >
            <button className={!cat ? "active" : ""} onClick={() => setCat("")}>
              {t("All")} <small>{cards.length}</small>
            </button>
            {cats(cards).map((c) => (
              <button
                key={c.id}
                className={cat === c.id ? "active" : ""}
                onClick={() => setCat(c.id)}
              >
                {t(c.label)} <small>{c.items.length}</small>
              </button>
            ))}
          </nav>
        }
        end={
          <span className="xc-muted habits-rec-count">
            {t("In habits")} {added} / {cards.length}
          </span>
        }
      />
      {shown.map((group) => (
        <section key={group.id} aria-label={t(group.label)}>
          {!cat && <h2 className="habits-rec-title">{t(group.label)}</h2>}
          <div className="habits-rec-grid">
            {group.items.map((c) => (
              <RecCard
                key={c.key}
                card={c}
                pending={pending}
                onEdit={setEditing}
                onRestore={(h) =>
                  restore.mutate(
                    { id: h.id, body: { archived: false } },
                    { onSuccess: () => toast(t("Restored")) },
                  )
                }
              />
            ))}
          </div>
        </section>
      ))}
      <HabitDialog
        open={draft !== null || editing !== null}
        habit={editing}
        initialTemplate={draft?.template}
        initialInput={draft?.input}
        existingTemplates={usedTemplates}
        onClose={() => {
          setDraft(null);
          setEditing(null);
        }}
      />
    </div>
  );
}

function RecCard({
  card: c,
  pending,
  onEdit,
  onRestore,
}: {
  card: Card;
  pending: boolean;
  onEdit: (h: Habit) => void;
  onRestore: (h: Habit) => void;
}) {
  const t = useT();
  return (
    <div className={`xc-card habits-rec-card${c.habit ? " is-added" : ""}`}>
      <span className="habits-rec-icon" aria-hidden>
        {c.icon || "✨"}
      </span>
      <div className="habits-rec-main">
        <strong>{c.name}</strong>
        <p title={c.summary}>{c.summary}</p>
        {c.meta && <small>{c.meta}</small>}
      </div>
      <div className="habits-rec-actions">
        {c.habit ? (
          <button
            className="xc-btn small ghost habits-rec-added"
            title={t("Already added. Click to edit.")}
            onClick={() => onEdit(c.habit!)}
          >
            <Check size={13} /> {t("In habits")}
          </button>
        ) : c.archived ? (
          <button
            className="xc-btn small"
            disabled={pending}
            onClick={() => onRestore(c.archived!)}
          >
            {t("Restore")}
          </button>
        ) : (
          <button
            className="xc-btn small"
            disabled={pending}
            aria-label={`${t("Add")}：${c.name}`}
            onClick={c.add}
          >
            <Plus size={13} /> {t("Add")}
          </button>
        )}
        {c.program && (
          <Link
            className="xc-btn small ghost"
            to={`/habits/fitness?program=${c.program}`}
          >
            <Dumbbell size={13} /> {t("See exercises")}
          </Link>
        )}
      </div>
    </div>
  );
}

function base(r: Recommendation) {
  return {
    key: r.id,
    category: r.category,
    name: r.name,
    icon: r.icon,
    summary: r.summary,
  };
}

/** 按分类分组，保持分类本来的顺序，空的分类不显示。 */
function cats(cards: Card[]) {
  return recommendCategories
    .map((c) => ({ ...c, items: cards.filter((x) => x.category === c.id) }))
    .filter((c) => c.items.length > 0);
}

/** 个人计划打卡模板的图标。服务端的模板没有图标，这里按 id 配。 */
const libraryIcons: Record<string, string> = {
  weight: "⚖️",
  breakfast: "🥚",
  words: "🔤",
  eyes: "👀",
  stand: "🚶",
  desk: "🪑",
  lunch: "🥡",
  steps: "👟",
  core: "🎯",
  protein: "🍗",
  review: "🔁",
  sleep: "🌙",
};

/** “每天 8 杯 · 每 60 分钟提醒”这样一行。 */
function remindMeta(
  v: Omit<HabitInput, "activeHostIds">,
  t: (s: string) => string,
) {
  const goal = `${t("Daily")} ${v.dailyTarget ?? 1} ${v.unit ?? "次"}`;
  if (v.remindMode === "interval")
    return `${goal} · ${t("Every")} ${v.remindIntervalMinutes} ${t("minutes")}`;
  if (v.remindMode === "times" && v.remindTimes?.length)
    return `${goal} · ${v.remindTimes.join("、")}`;
  return goal;
}

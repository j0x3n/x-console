import {
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import {
  Archive,
  ArrowRightLeft,
  Bot,
  CalendarClock,
  Check,
  ChevronLeft,
  ChevronRight,
  Copy,
  ExternalLink,
  Link2,
  Palette,
  Tag,
  UserRound,
} from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useAiAgents } from "../../aiagents/api";
import AgentAvatar from "../../aiagents/AgentAvatar";
import {
  useCardActions,
  useLabels,
  useUpdateIssue,
  type Board,
  type CardColor,
  type Issue,
} from "../api";
import { hasMember, issuePath, toggleMember } from "../logic";
import { dueShortcuts, CARD_COLORS } from "../cardMenu";

type Panel = "main" | "assign" | "labels" | "color" | "due" | "move";

const EDGE = 8;

/**
 * B85：看板上右键一张卡片（手机上长按）弹出的菜单，参考 Trello 的快速编辑。
 * 子菜单在同一个浮层里切换，左上角“返回”回到上一级，手机上也好点。
 */
export default function CardMenu({
  issue,
  board,
  at,
  onClose,
  onOpen,
  onMoveToList,
  onStartAgent,
}: {
  issue: Issue;
  board: Board;
  /** 鼠标的位置 */
  at: { x: number; y: number };
  onClose: () => void;
  onOpen: () => void;
  onMoveToList: (listId: number) => void;
  /** 让 Agent 开始做：打开分配弹窗 */
  onStartAgent: () => void;
}) {
  const t = useT();
  const [panel, setPanel] = useState<Panel>("main");
  const [style, setStyle] = useState<CSSProperties>({ visibility: "hidden" });
  const ref = useRef<HTMLDivElement>(null);
  const labels = useLabels(issue.projectId);
  const agents = useAiAgents();
  const cards = useCardActions();
  const update = useUpdateIssue();
  const [customDue, setCustomDue] = useState("");

  // 放在鼠标处；超出窗口时往回挪。手机上由样式放到底部。
  useLayoutEffect(() => {
    const m = ref.current?.getBoundingClientRect();
    if (!m) return;
    if (window.matchMedia?.("(max-width: 640px)").matches) {
      setStyle({});
      return;
    }
    setStyle({
      left: Math.max(EDGE, Math.min(at.x, window.innerWidth - EDGE - m.width)),
      top: Math.max(EDGE, Math.min(at.y, window.innerHeight - EDGE - m.height)),
    });
  }, [at.x, at.y, panel]);

  const save = (body: Parameters<typeof update.mutate>[0]["body"]) =>
    update.mutate({ issue, key: issue.key, body });
  const done = (fn: () => void) => () => {
    fn();
    onClose();
  };
  const labelIds = issue.labels.map((l) => l.id);
  // 卡片颜色接口还没上线时（返回的卡片没有 color 字段）不显示这一项
  const colorLive = issue.color !== undefined;

  let body: ReactNode;
  if (panel === "assign")
    body = (
      <>
        <Item
          icon={<UserRound size={14} />}
          checked={hasMember(issue, "me")}
          onClick={() =>
            cards.members.mutate({
              key: issue.key,
              members: toggleMember(issue, "me"),
            })
          }
        >
          {t("Me")}
        </Item>
        {(agents.data ?? []).map((a) => (
          <Item
            key={a.id}
            icon={<AgentAvatar agent={a} size={16} />}
            checked={hasMember(issue, "agent", String(a.id))}
            onClick={() =>
              cards.members.mutate({
                key: issue.key,
                members: toggleMember(issue, "agent", String(a.id)),
              })
            }
          >
            {a.name}
          </Item>
        ))}
        <div className="projects-cardmenu-sep" />
        <Item icon={<Bot size={14} />} onClick={done(onStartAgent)}>
          {t("Let an agent work on it…")}
        </Item>
      </>
    );
  else if (panel === "labels")
    body =
      (labels.data ?? []).length === 0 ? (
        <p className="projects-cardmenu-note">{t("No labels yet")}</p>
      ) : (
        (labels.data ?? []).map((l) => (
          <Item
            key={l.id}
            icon={
              <i
                className="projects-cardmenu-swatch"
                style={{ background: l.color }}
              />
            }
            checked={labelIds.includes(l.id)}
            onClick={() =>
              save({
                labelIds: labelIds.includes(l.id)
                  ? labelIds.filter((id) => id !== l.id)
                  : [...labelIds, l.id],
              })
            }
          >
            {l.name}
          </Item>
        ))
      );
  else if (panel === "color")
    body = (
      <>
        <div className="projects-cardmenu-colors">
          {CARD_COLORS.map((c) => (
            <button
              key={c}
              type="button"
              role="menuitemradio"
              aria-checked={issue.color === c}
              aria-label={t(`Color ${c}`)}
              title={t(`Color ${c}`)}
              className={`projects-cardmenu-color card-color-${c}${issue.color === c ? " on" : ""}`}
              onClick={() =>
                save({ color: (issue.color === c ? "" : c) as CardColor })
              }
            />
          ))}
        </div>
        {issue.color && (
          <Item onClick={() => save({ color: "" })}>{t("Remove color")}</Item>
        )}
      </>
    );
  else if (panel === "due")
    body = (
      <>
        {dueShortcuts(new Date()).map((d) => (
          <Item
            key={d.label}
            onClick={done(() => save({ dueAt: d.at.toISOString() }))}
          >
            {t(d.label)}
            <small>{d.hint}</small>
          </Item>
        ))}
        <form
          className="projects-cardmenu-custom"
          onSubmit={(e) => {
            e.preventDefault();
            if (!customDue) return;
            save({ dueAt: new Date(customDue).toISOString() });
            onClose();
          }}
        >
          <input
            type="datetime-local"
            className="xc-input"
            aria-label={t("Pick a date and time")}
            value={customDue}
            onChange={(e) => setCustomDue(e.target.value)}
          />
          <button className="xc-btn small primary" disabled={!customDue}>
            {t("Save")}
          </button>
        </form>
        {issue.dueAt && (
          <Item onClick={done(() => save({ dueAt: null }))}>
            {t("Clear due time")}
          </Item>
        )}
      </>
    );
  else if (panel === "move")
    body = (
      <>
        {board.lists
          .filter((l) => !l.archivedAt)
          .map((l) => (
            <Item
              key={l.id}
              checked={issue.listId === l.id}
              onClick={done(() => {
                if (issue.listId !== l.id) onMoveToList(l.id);
              })}
            >
              {l.name}
            </Item>
          ))}
        <div className="projects-cardmenu-sep" />
        <Item icon={<ExternalLink size={14} />} onClick={done(onOpen)}>
          {t("Move to another board…")}
        </Item>
      </>
    );
  else
    body = (
      <>
        <Item icon={<ExternalLink size={14} />} onClick={done(onOpen)}>
          {t("Open card")}
        </Item>
        <Item
          icon={<UserRound size={14} />}
          more
          onClick={() => setPanel("assign")}
        >
          {t("Assign")}
        </Item>
        <Item icon={<Tag size={14} />} more onClick={() => setPanel("labels")}>
          {t("Labels")}
        </Item>
        {colorLive && (
          <Item
            icon={<Palette size={14} />}
            more
            onClick={() => setPanel("color")}
          >
            {t("Color")}
          </Item>
        )}
        <Item
          icon={<CalendarClock size={14} />}
          more
          onClick={() => setPanel("due")}
        >
          {t("Set due time")}
        </Item>
        <Item
          icon={<ArrowRightLeft size={14} />}
          more
          onClick={() => setPanel("move")}
        >
          {t("Move")}
        </Item>
        <Item
          icon={<Copy size={14} />}
          onClick={done(() =>
            cards.copy.mutate(issue.key, {
              onSuccess: () => toast(t("Card copied")),
            }),
          )}
        >
          {t("Copy card")}
        </Item>
        <Item
          icon={<Link2 size={14} />}
          onClick={done(() => {
            const url = new URL(issuePath(issue.key), location.origin).href;
            navigator.clipboard
              ?.writeText(url)
              .then(() => toast(t("Link copied")))
              .catch(() => toast({ message: url, tone: "error" }));
          })}
        >
          {t("Copy link")}
        </Item>
        <div className="projects-cardmenu-sep" />
        <Item
          icon={<Archive size={14} />}
          onClick={done(() =>
            cards.archive.mutate(issue.key, {
              onSuccess: () =>
                toast({
                  message: t("Card archived"),
                  onUndo: () => cards.restore.mutate(issue.key),
                }),
            }),
          )}
        >
          {t("Archive")}
        </Item>
      </>
    );

  return createPortal(
    <>
      <div
        className="xc-more-backdrop"
        onClick={onClose}
        onContextMenu={(e) => {
          e.preventDefault();
          onClose();
        }}
      />
      <div
        ref={ref}
        className="xc-more-menu projects-cardmenu"
        role="menu"
        aria-label={issue.title}
        style={style}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.stopPropagation();
            if (panel === "main") onClose();
            else setPanel("main");
          }
        }}
      >
        <div className="projects-cardmenu-head">
          {panel !== "main" && (
            <button
              type="button"
              className="projects-cardmenu-back"
              aria-label={t("Back")}
              onClick={() => setPanel("main")}
            >
              <ChevronLeft size={14} />
            </button>
          )}
          <span>{panel === "main" ? issue.title : t(PANEL_TITLES[panel])}</span>
        </div>
        {body}
      </div>
    </>,
    document.body,
  );
}

const PANEL_TITLES: Record<Exclude<Panel, "main">, string> = {
  assign: "Assign",
  labels: "Labels",
  color: "Color",
  due: "Set due time",
  move: "Move",
};

function Item({
  icon,
  checked,
  more,
  onClick,
  children,
}: {
  icon?: ReactNode;
  checked?: boolean;
  more?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role={checked === undefined ? "menuitem" : "menuitemcheckbox"}
      aria-checked={checked}
      onClick={onClick}
    >
      {icon}
      <span className="projects-cardmenu-label">{children}</span>
      {checked && <Check size={14} className="projects-cardmenu-check" />}
      {more && <ChevronRight size={14} className="projects-cardmenu-more" />}
    </button>
  );
}

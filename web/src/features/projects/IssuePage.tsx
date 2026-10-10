import { useEffect, useState, type ReactNode } from "react";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { usePageCrumb } from "../../stores/page-title";
import {
  Archive,
  ArchiveRestore,
  Bot,
  CircleCheck,
  Copy,
  ExternalLink,
  FolderGit2,
  GitPullRequest,
  Link2,
  Maximize2,
  NotebookPen,
  Plus,
  Trash2,
  X,
} from "lucide-react";
import { ApiError } from "../../api/client";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useAddComment,
  useAddLink,
  useB36Live,
  useBoards,
  useCardActions,
  useComments,
  useIssueActivity,
  useMoveCard,
  useDeleteComment,
  useDeleteIssue,
  useDeleteLink,
  useIssue,
  useLabels,
  useLinks,
  useMilestones,
  useUpdateIssue,
  type Issue,
  type IssueLink,
  type UpdateIssue,
} from "./api";
import { LabelChip, PriorityIcon, StatusIcon } from "./components/Icons";
import { ListSelect } from "./components/BoardDialogs";
import MenuPick from "./components/MenuPick";
import Checklists from "./components/Checklists";
import DueFields from "./components/DueFields";
import Markdown from "./Markdown";
import StartFocusButton from "../calendar/StartFocusButton";
import MoreMenu from "../../components/ui/MoreMenu";
import AssignDialog from "../aiagents/AssignDialog";
import AgentMember from "../aiagents/AgentMember";
import AgentRunLog from "../aiagents/AgentRunLog";
import BindRepoDialog from "./components/BindRepoDialog";
import {
  PRIORITIES,
  PRIORITY_LABELS,
  STATUSES,
  STATUS_LABELS,
  hasMember,
  issueDue,
  issueDueState,
  issuePath,
  joinDue,
  overdueBy,
  splitDue,
  toggleMember,
} from "./logic";
import { useShortcuts } from "./useShortcuts";
import { confirmAction } from "../../components/ui/ConfirmDialog";

export default function IssuePage() {
  const navigate = useNavigate();
  const { projectKey = "", number = "" } = useParams();
  const key = `${projectKey.toUpperCase()}-${number}`;
  const issue = useIssue(key);
  usePageCrumb(
    issue.data?.key ?? "",
    issue.data
      ? [
          {
            label: issue.data.projectKey,
            to: `/projects/${issue.data.projectKey}`,
          },
        ]
      : undefined,
  );
  return (
    <IssueView
      issueKey={key}
      onClose={() => navigate(`/projects/${projectKey.toUpperCase()}`)}
    />
  );
}

/** 看板里点卡片时，在右侧打开同一份详情。 */
export function IssuePanel({
  issueKey,
  onClose,
  editNonce = 0,
}: {
  issueKey: string;
  onClose: () => void;
  editNonce?: number;
}) {
  return (
    <IssueView issueKey={issueKey} panel onClose={onClose} editNonce={editNonce} />
  );
}

function IssueView({
  issueKey,
  panel,
  onClose,
  editNonce = 0,
}: {
  issueKey: string;
  panel?: boolean;
  onClose: () => void;
  editNonce?: number;
}) {
  const t = useT();
  const issue = useIssue(issueKey);
  const update = useUpdateIssue();
  const [search, setSearch] = useSearchParams();
  const [editingTitle, setEditingTitle] = useState(
    !panel && search.get("edit") === "title",
  );
  const [addingChecklist, setAddingChecklist] = useState(false);

  useEffect(() => {
    if (panel || search.get("edit") !== "title") return;
    setEditingTitle(true);
    search.delete("edit");
    setSearch(search, { replace: true });
  }, [panel, search, setSearch]);
  useEffect(() => {
    if (panel && editNonce > 0) setEditingTitle(true);
  }, [panel, editNonce]);

  const save = (body: UpdateIssue) =>
    issue.data &&
    update.mutate({ issue: issue.data, key: issue.data.key, body });

  const frame = (content: ReactNode) =>
    panel ? (
      <div className="projects-drawer" role="dialog" aria-label={issueKey}>
        <header className="projects-drawer-head">
          <span className="projects-drawer-key">{issueKey}</span>
          <span className="xc-spacer" />
          <Link
            className="xc-btn ghost small"
            to={issuePath(issueKey)}
            title={t("Open the detail page")}
            aria-label={t("Open the detail page")}
          >
            <ExternalLink size={14} />
          </Link>
          <button
            type="button"
            className="icon-button"
            aria-label={t("Close")}
            title={t("Close")}
            onClick={onClose}
          >
            <X size={16} />
          </button>
        </header>
        <div className="projects-drawer-body">{content}</div>
      </div>
    ) : (
      <div className="xc-page wide projects-issue-page">{content}</div>
    );

  useShortcuts(
    {
      e: () => setEditingTitle(true),
      escape: () => onClose(),
      ...Object.fromEntries(
        STATUSES.map((status, i) => [
          String(i + 1),
          () => {
            if (issue.data && issue.data.status !== status) save({ status });
          },
        ]),
      ),
    },
    !!issue.data,
  );

  if (issue.isPending) return frame(<Loading />);
  if (issue.isError) {
    return frame(
      issue.error instanceof ApiError && issue.error.status === 404 ? (
        <EmptyState title={t("Issue not found")}>
          {panel ? (
            <button className="xc-btn small" onClick={onClose}>
              {t("Close")}
            </button>
          ) : (
            <Link to={`/projects/${issueKey.split("-")[0]}`}>
              {t("Back to project")}
            </Link>
          )}
        </EmptyState>
      ) : (
        <ErrorState error={issue.error} onRetry={() => issue.refetch()} />
      ),
    );
  }
  const data = issue.data;
  if (!data) return frame(<Loading />);
  if (panel) {
    return (
      <div className="projects-drawer" role="dialog" aria-label={data.key}>
        <header className="projects-drawer-head">
          <span className="projects-drawer-proj">{data.projectKey}</span>
          <span className="projects-drawer-sep">/</span>
          <span className="projects-drawer-key">{data.key}</span>
          <span className="xc-spacer" />
          <button
            type="button"
            className={`xc-btn ghost small${data.status === "done" ? " projects-drawer-done" : ""}`}
            onClick={() =>
              save({ status: data.status === "done" ? "todo" : "done" })
            }
          >
            <CircleCheck size={14} />
            {data.status === "done" ? t("Reopen") : t("Mark complete")}
          </button>
          <button
            type="button"
            className="projects-drawer-icon"
            aria-label={t("Copy link")}
            title={t("Copy link")}
            onClick={() => copyIssueLink(data.key, t)}
          >
            <Link2 size={15} />
          </button>
          <IssueTools issue={data} />
          <Link
            className="projects-drawer-icon"
            to={issuePath(data.key)}
            title={t("Open the detail page")}
            aria-label={t("Open the detail page")}
          >
            <Maximize2 size={15} />
          </Link>
          <button
            type="button"
            className="projects-drawer-icon"
            aria-label={t("Close")}
            title={t("Close")}
            onClick={onClose}
          >
            <X size={16} />
          </button>
        </header>
        <div className="projects-drawer-body">
          <DrawerNotice issue={data} onSave={save} />
          <IssueTitle
            issue={data}
            editing={editingTitle}
            setEditing={setEditingTitle}
            onSave={(title) => save({ title })}
            plain
          />
          <Properties issue={data} onSave={save} rows />
          <Description
            issue={data}
            headed
            onSave={(description) => save({ description })}
          />
          <Checklists
            issueKey={data.key}
            adding={addingChecklist}
            onAddingChange={setAddingChecklist}
          />
          <Links issueKey={data.key} />
          <AgentRuns issueKey={data.key} />
          <DrawerFeed issueKey={data.key} />
          <QuietTimes issue={data} />
        </div>
      </div>
    );
  }
  return frame(
    <>
      {(data.externalSource || data.archivedAt) && (
        <nav className="projects-issue-crumbs">
          {data.externalSource && (
            <span className="xc-badge info">{data.externalSource}</span>
          )}
          {data.archivedAt && (
            <span className="xc-badge warn">{t("Archived")}</span>
          )}
        </nav>
      )}
      <div className="projects-issue-layout">
        <main className="projects-issue-main">
          <IssueTitle
            issue={data}
            editing={editingTitle}
            setEditing={setEditingTitle}
            onSave={(title) => save({ title })}
          />
          <Description
            issue={data}
            onSave={(description) => save({ description })}
          />
          <Checklists
            issueKey={data.key}
            adding={addingChecklist}
            onAddingChange={setAddingChecklist}
          />
          <Links issueKey={data.key} />
          <AgentRuns issueKey={data.key} />
          <Comments issueKey={data.key} />
          <Activity issueKey={data.key} />
          <QuietTimes issue={data} />
        </main>
        <aside className="projects-issue-side">
          <Properties issue={data} onSave={save} />
        </aside>
      </div>
    </>,
  );
}

function QuietTimes({ issue }: { issue: Issue }) {
  const t = useT();
  const language = useLanguage();
  return (
    <p className="projects-quiet-times">
      <span title={issue.createdAt}>
        {t("Created")} {relativeTime(issue.createdAt, language)}
      </span>
      <span title={issue.updatedAt}>
        {t("Updated")} {relativeTime(issue.updatedAt, language)}
      </span>
      {issue.completedAt && (
        <span title={issue.completedAt}>
          {t("Completed")} {relativeTime(issue.completedAt, language)}
        </span>
      )}
    </p>
  );
}

function copyIssueLink(key: string, t: (s: string) => string) {
  const url = new URL(issuePath(key), location.origin).href;
  navigator.clipboard
    ?.writeText(url)
    .then(() => toast(t("Link copied")))
    .catch(() => toast({ message: url, tone: "error" }));
}

function DrawerNotice({
  issue,
  onSave,
}: {
  issue: Issue;
  onSave: (body: UpdateIssue) => void;
}) {
  const t = useT();
  if (issue.status === "done") {
    return (
      <p className="projects-drawer-note ok">
        <CircleCheck size={15} />
        <span>{t("This card is done")}</span>
        <button
          type="button"
          className="xc-btn ghost small"
          onClick={() => onSave({ status: "todo" })}
        >
          {t("Reopen")}
        </button>
      </p>
    );
  }
  const at = issueDue(issue);
  if (issueDueState(issue, new Date()) !== "overdue" || !at) return null;
  const late = overdueBy(at, new Date());
  return (
    <p className="projects-drawer-note danger">
      <span>
        {t("Overdue for")} {late.value} {t(late.unit)}
      </span>
    </p>
  );
}

function DrawerFeed({ issueKey }: { issueKey: string }) {
  const t = useT();
  const comments = useComments(issueKey);
  const activity = useIssueActivity(issueKey);
  const [tab, setTab] = useState<"comments" | "activity">("comments");
  return (
    <section className="projects-drawer-feed">
      <div className="projects-drawer-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={tab === "comments"}
          className={tab === "comments" ? "on" : ""}
          onClick={() => setTab("comments")}
        >
          {t("Comments")}
          <span>{comments.data?.length ?? 0}</span>
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === "activity"}
          className={tab === "activity" ? "on" : ""}
          onClick={() => setTab("activity")}
        >
          {t("Activity")}
          <span>{activity.data?.length ?? 0}</span>
        </button>
      </div>
      {tab === "comments" ? (
        <Comments issueKey={issueKey} bare />
      ) : (
        <Activity issueKey={issueKey} bare />
      )}
    </section>
  );
}

function IssueTitle({
  issue,
  editing,
  setEditing,
  onSave,
  plain,
}: {
  issue: Issue;
  editing: boolean;
  setEditing: (v: boolean) => void;
  onSave: (title: string) => void;
  plain?: boolean;
}) {
  const t = useT();
  const [value, setValue] = useState(issue.title);
  useEffect(() => {
    if (!editing) setValue(issue.title);
  }, [issue.title, editing]);
  const commit = () => {
    setEditing(false);
    const title = value.trim();
    if (title && title !== issue.title) onSave(title);
    else setValue(issue.title);
  };
  if (editing)
    return (
      <input
        className={plain ? "projects-ttl" : "xc-input projects-title-input"}
        value={value}
        autoFocus
        aria-label={t("Title")}
        onChange={(e) => setValue(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter") commit();
          if (e.key === "Escape") {
            e.stopPropagation();
            setValue(issue.title);
            setEditing(false);
          }
        }}
      />
    );
  return (
    <h1
      className={plain ? "projects-ttl" : "projects-issue-title"}
      onClick={() => setEditing(true)}
      title={t("Click or press E to edit")}
    >
      {issue.title}
    </h1>
  );
}

function Description({
  issue,
  onSave,
  headed,
}: {
  issue: Issue;
  onSave: (description: string) => void;
  headed?: boolean;
}) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(issue.description);
  useEffect(() => {
    if (!editing) setValue(issue.description);
  }, [issue.description, editing]);
  const commit = () => {
    if (value !== issue.description) onSave(value);
    setEditing(false);
  };
  const body = !editing ? (
    <section
      className="projects-description"
      onDoubleClick={() => setEditing(true)}
    >
      <Markdown
        source={issue.description}
        empty={
          <button
            className="projects-placeholder"
            onClick={() => setEditing(true)}
          >
            {t("Add a description…")}
          </button>
        }
      />
      {issue.description && (
        <button
          className="xc-btn ghost small"
          onClick={() => setEditing(true)}
        >
          {t("Edit description")}
        </button>
      )}
    </section>
  ) : (
    <section
      className="projects-description editing"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.stopPropagation();
          setValue(issue.description);
          setEditing(false);
        }
      }}
    >
      <MarkdownEditor
        polish="card"
        uploadScope="projects"
        label={t("Description")}
        value={value}
        onChange={setValue}
        autoFocus
        minRows={headed ? 6 : 10}
        placeholder={t("Markdown is supported")}
        onSubmit={commit}
      />
      <div className="xc-row projects-md-actions">
        <span className="xc-muted projects-hint">⌘/Ctrl + Enter</span>
        <span className="xc-spacer" />
        <button
          className="xc-btn small"
          onClick={() => {
            setValue(issue.description);
            setEditing(false);
          }}
        >
          {t("Cancel")}
        </button>
        <button className="xc-btn primary small" onClick={commit}>
          {t("Save")}
        </button>
      </div>
    </section>
  );
  if (!headed) return body;
  return (
    <section className="projects-dsec">
      <header>
        <h2>{t("Description")}</h2>
      </header>
      {body}
    </section>
  );
}

function Properties({
  issue,
  onSave,
  rows,
}: {
  issue: Issue;
  onSave: (body: UpdateIssue) => void;
  rows?: boolean;
}) {
  const t = useT();
  const navigate = useNavigate();
  const labels = useLabels(issue.projectId);
  const milestones = useMilestones(issue.projectId);
  const { live } = useB36Live(issue.projectId);
  const boards = useBoards(issue.projectId);
  const move = useMoveCard();
  const cards = useCardActions();
  const remove = useDeleteIssue();
  const [assigning, setAssigning] = useState(false);
  const [binding, setBinding] = useState(false);
  // B86：卡片所在看板绑定的仓库，分配给 Agent 时默认用它
  const board =
    boards.data?.find((b) => b.id === issue.boardId) ?? boards.data?.[0];
  const labelIds = issue.labels.map((l) => l.id);
  return (
    <div className={rows ? "projects-props projects-props-rows" : "projects-props"}>
      <label className="projects-prop">
        <span>{t("Status")}</span>
        <div className="projects-prop-control">
          <MenuPick
            label={t("Status")}
            value={issue.status}
            options={STATUSES.map((s) => ({
              value: s,
              label: t(STATUS_LABELS[s]),
              icon: <StatusIcon status={s} size={14} />,
            }))}
            onChange={(status) => onSave({ status })}
          />
        </div>
      </label>
      <label className="projects-prop">
        <span>{t("Priority")}</span>
        <div className="projects-prop-control">
          <MenuPick
            label={t("Priority")}
            value={issue.priority}
            empty={issue.priority === 0}
            options={PRIORITIES.map((p) => ({
              value: p,
              label: t(PRIORITY_LABELS[p]),
              icon: <PriorityIcon priority={p} size={14} />,
            }))}
            onChange={(priority) => onSave({ priority })}
          />
        </div>
      </label>
      {boards.data && issue.listId !== undefined && (
        <label className="projects-prop">
          <span>{t("Board and list")}</span>
          <ListSelect
            label={t("Board and list")}
            boards={boards.data}
            value={issue.listId}
            menu
            onChange={(listId) => {
              const list = boards.data
                .flatMap((b) => b.lists)
                .find((l) => l.id === listId);
              move.mutate({
                projectId: issue.projectId,
                key: issue.key,
                plan: { listId, status: list?.status, sortOrder: null },
              });
            }}
          />
        </label>
      )}
      <div className="projects-prop">
        <span>{t("Members")}</span>
        <div className="projects-label-picker">
          <button
            className={hasMember(issue, "me") ? "on" : ""}
            aria-pressed={hasMember(issue, "me")}
            onClick={() =>
              cards.members.mutate({
                key: issue.key,
                members: toggleMember(issue, "me"),
              })
            }
          >
            <i className="projects-member me">{t("Me")}</i>
            {t("Me")}
          </button>
          {(issue.members ?? [])
            .filter((m) => m.kind === "agent")
            .map((m) => (
              <button
                key={m.id}
                className="on"
                aria-pressed
                title={t("Remove from the card")}
                onClick={() =>
                  cards.members.mutate({
                    key: issue.key,
                    members: toggleMember(issue, "agent", m.id),
                  })
                }
              >
                <AgentMember id={m.id} issueKey={issue.key} withName />
              </button>
            ))}
        </div>
      </div>
      <div className="projects-prop">
        <span>{live ? t("Due at") : t("Due date")}</span>
        <DueFields
          live={live}
          menu
          value={splitDue(issue)}
          onChange={(v) =>
            // 后端没上线 B36 时只认 dueDate，多发字段会被拒绝。
            onSave(
              live
                ? { dueAt: joinDue(v.date, v.time) }
                : { dueDate: v.date || null },
            )
          }
          remind={issue.dueRemind}
          onRemindChange={(dueRemind) => onSave({ dueRemind })}
        />
      </div>
      <label className="projects-prop">
        <span>{t("Milestone")}</span>
        <MenuPick
          label={t("Milestone")}
          value={issue.milestoneId ?? 0}
          empty={!issue.milestoneId}
          options={[
            { value: 0, label: t("None") },
            ...(milestones.data ?? []).map((m) => ({
              value: m.id,
              label: m.name,
            })),
          ]}
          onChange={(id) => onSave({ milestoneId: id || null })}
        />
      </label>
      <div className="projects-prop">
        <span>{t("Labels")}</span>
        <div className="projects-label-picker">
          {labels.data?.map((label) => {
            const on = labelIds.includes(label.id);
            return (
              <button
                key={label.id}
                className={on ? "on" : ""}
                aria-pressed={on}
                onClick={() =>
                  onSave({
                    labelIds: on
                      ? labelIds.filter((id) => id !== label.id)
                      : [...labelIds, label.id],
                  })
                }
              >
                <LabelChip label={label} />
              </button>
            );
          })}
          {labels.data?.length === 0 && (
            <span className="xc-muted">{t("No labels yet")}</span>
          )}
        </div>
      </div>
      {rows ? null : (
      <>
      <div className="projects-side-actions">
        <button
          className="xc-btn projects-side-primary"
          onClick={() => setAssigning(true)}
        >
          <Bot size={14} /> {t("Assign to an agent")}
        </button>
        <StartFocusButton issueKey={issue.key} compact />
        {issue.externalUrl && (
          // B84：从仓库同步来的卡片
          <a
            className="xc-btn"
            href={issue.externalUrl}
            target="_blank"
            rel="noreferrer"
            title={t("Open in repository")}
            aria-label={t("Open in repository")}
          >
            <FolderGit2 size={14} />
          </a>
        )}
        <MoreMenu
          label={`${t("More")}：${issue.key}`}
          title={issue.key}
          items={[
            {
              key: "coding",
              label: t("New coding task by hand"),
              icon: <Bot size={14} />,
              onSelect: () =>
                navigate(
                  `/coding/tasks?new=1&issue=${encodeURIComponent(issue.key)}`,
                ),
            },
            {
              key: "copy",
              label: t("Copy card"),
              icon: <Copy size={14} />,
              onSelect: () =>
                cards.copy.mutate(issue.key, {
                  onSuccess: (copy) => navigate(issuePath(copy.key)),
                }),
            },
            issue.archivedAt
              ? {
                  key: "restore",
                  label: t("Restore"),
                  icon: <ArchiveRestore size={14} />,
                  onSelect: () => cards.restore.mutate(issue.key),
                }
              : {
                  key: "archive",
                  label: t("Archive"),
                  icon: <Archive size={14} />,
                  onSelect: () => cards.archive.mutate(issue.key),
                },
            {
              key: "delete",
              label: t("Delete issue"),
              icon: <Trash2 size={14} />,
              danger: true,
              onSelect: async () => {
                if (
                  !(await confirmAction({
                    title: `${t("Delete")} ${issue.key}？`,
                    description: t("Its comments and links are deleted too."),
                  }))
                )
                  return;
                remove.mutate(issue.key, {
                  onSuccess: () => navigate(`/projects/${issue.projectKey}`),
                });
              },
            },
          ]}
        />
      </div>
      {assigning && (
        <AssignDialog
          issueKey={issue.key}
          boardRepo={board?.repo}
          onBindRepo={
            board
              ? () => {
                  setAssigning(false);
                  setBinding(true);
                }
              : undefined
          }
          onClose={() => setAssigning(false)}
        />
      )}
      {binding && board && (
        <BindRepoDialog board={board} onClose={() => setBinding(false)} />
      )}
      </>
      )}
    </div>
  );
}

function IssueTools({ issue }: { issue: Issue }) {
  const t = useT();
  const navigate = useNavigate();
  const boards = useBoards(issue.projectId);
  const cards = useCardActions();
  const remove = useDeleteIssue();
  const [assigning, setAssigning] = useState(false);
  const [binding, setBinding] = useState(false);
  const board =
    boards.data?.find((b) => b.id === issue.boardId) ?? boards.data?.[0];
  return (
    <>
      <button
        type="button"
        className="projects-drawer-icon"
        aria-label={t("Assign to an agent")}
        title={t("Assign to an agent")}
        onClick={() => setAssigning(true)}
      >
        <Bot size={15} />
      </button>
      <StartFocusButton issueKey={issue.key} compact />
      {issue.externalUrl && (
        <a
          className="projects-drawer-icon"
          href={issue.externalUrl}
          target="_blank"
          rel="noreferrer"
          title={t("Open in repository")}
          aria-label={t("Open in repository")}
        >
          <FolderGit2 size={15} />
        </a>
      )}
      <MoreMenu
        label={`${t("More")}：${issue.key}`}
        title={issue.key}
        items={[
          {
            key: "coding",
            label: t("New coding task by hand"),
            icon: <Bot size={14} />,
            onSelect: () =>
              navigate(
                `/coding/tasks?new=1&issue=${encodeURIComponent(issue.key)}`,
              ),
          },
          {
            key: "copy",
            label: t("Copy card"),
            icon: <Copy size={14} />,
            onSelect: () =>
              cards.copy.mutate(issue.key, {
                onSuccess: (copy) => navigate(issuePath(copy.key)),
              }),
          },
          issue.archivedAt
            ? {
                key: "restore",
                label: t("Restore"),
                icon: <ArchiveRestore size={14} />,
                onSelect: () => cards.restore.mutate(issue.key),
              }
            : {
                key: "archive",
                label: t("Archive"),
                icon: <Archive size={14} />,
                onSelect: () => cards.archive.mutate(issue.key),
              },
          {
            key: "delete",
            label: t("Delete issue"),
            icon: <Trash2 size={14} />,
            danger: true,
            onSelect: async () => {
              if (
                !(await confirmAction({
                  title: `${t("Delete")} ${issue.key}？`,
                  description: t("Its comments and links are deleted too."),
                }))
              )
                return;
              remove.mutate(issue.key, {
                onSuccess: () => navigate(`/projects/${issue.projectKey}`),
              });
            },
          },
        ]}
      />
      {assigning && (
        <AssignDialog
          issueKey={issue.key}
          boardRepo={board?.repo}
          onBindRepo={
            board
              ? () => {
                  setAssigning(false);
                  setBinding(true);
                }
              : undefined
          }
          onClose={() => setAssigning(false)}
        />
      )}
      {binding && board && (
        <BindRepoDialog board={board} onClose={() => setBinding(false)} />
      )}
    </>
  );
}

const linkIcons = {
  pull_request: GitPullRequest,
  coding_task: Bot,
  note: NotebookPen,
  url: Link2,
};

function Links({ issueKey }: { issueKey: string }) {
  const t = useT();
  const links = useLinks(issueKey);
  const add = useAddLink(issueKey);
  const remove = useDeleteLink(issueKey);
  const [adding, setAdding] = useState(false);
  const [url, setUrl] = useState("");
  const [title, setTitle] = useState("");
  const submit = () => {
    if (!url.trim()) return;
    add.mutate(
      { kind: "url", url: url.trim(), title: title.trim() || undefined },
      {
        onSuccess: () => {
          setUrl("");
          setTitle("");
          setAdding(false);
        },
      },
    );
  };
  return (
    <section className="projects-section">
      <header>
        <h2>{t("Links")}</h2>
        <button
          className="xc-btn ghost small"
          onClick={() => setAdding((v) => !v)}
        >
          <Plus size={14} /> {t("Add link")}
        </button>
      </header>
      {links.data?.length === 0 && !adding && (
        <p className="xc-muted projects-section-empty">
          {t("Pull requests, coding tasks and notes show up here.")}
        </p>
      )}
      <ul className="projects-links">
        {links.data?.map((link) => (
          <LinkRow
            key={link.id}
            link={link}
            onDelete={async () =>
              (await confirmAction({ title: t("Delete this link?") })) &&
              remove.mutate(link.id)
            }
          />
        ))}
      </ul>
      {adding && (
        <form
          className="projects-inline-form"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <input
            className="xc-input"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://…"
            autoFocus
            aria-label="URL"
          />
          <input
            className="xc-input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={t("Title (optional)")}
            aria-label={t("Title")}
          />
          <button
            className="xc-btn small primary"
            disabled={!url.trim() || add.isPending}
          >
            {t("Add")}
          </button>
        </form>
      )}
    </section>
  );
}

function LinkRow({
  link,
  onDelete,
}: {
  link: IssueLink;
  onDelete: () => void;
}) {
  const t = useT();
  const Icon = linkIcons[link.kind] ?? Link2;
  const internal = link.url.startsWith("/");
  return (
    <li>
      <Icon size={15} />
      {internal ? (
        <Link to={link.url}>{link.title}</Link>
      ) : (
        <a href={link.url} target="_blank" rel="noreferrer noopener">
          {link.title} <ExternalLink size={12} />
        </a>
      )}
      {link.ref && <span className="xc-muted xc-mono">{link.ref}</span>}
      <span className="xc-spacer" />
      <button
        className="xc-btn ghost small"
        aria-label={t("Remove link")}
        onClick={onDelete}
      >
        <Trash2 size={13} />
      </button>
    </li>
  );
}

/** B86：这张卡片上 Agent 的执行记录和日志。没有记录时整节不显示。 */
function AgentRuns({ issueKey }: { issueKey: string }) {
  const t = useT();
  return (
    <section className="projects-section projects-agent-runs">
      <header>
        <h2>{t("Agent runs")}</h2>
      </header>
      <AgentRunLog issueKey={issueKey} hideWhenEmpty />
    </section>
  );
}

function Comments({
  issueKey,
  bare,
}: {
  issueKey: string;
  bare?: boolean;
}) {
  const t = useT();
  const language = useLanguage();
  const comments = useComments(issueKey);
  const add = useAddComment(issueKey);
  const remove = useDeleteComment(issueKey);
  const [body, setBody] = useState("");
  const submit = () => {
    if (!body.trim()) return;
    add.mutate(body, { onSuccess: () => setBody("") });
  };
  const list = (
    <>
      <ol className="projects-comments">
        {comments.data?.map((c) => (
          <li key={c.id}>
            <div className="projects-comment-head">
              {c.author?.startsWith("agent:") && (
                <AgentMember id={c.author.slice(6)} withName />
              )}
              <time dateTime={c.createdAt} title={c.createdAt}>
                {relativeTime(c.createdAt, language)}
              </time>
              <span className="xc-spacer" />
              <button
                className="xc-btn ghost small"
                aria-label={t("Delete comment")}
                onClick={async () =>
                  (await confirmAction({ title: t("Delete this comment?") })) &&
                  remove.mutate(c.id)
                }
              >
                <Trash2 size={13} />
              </button>
            </div>
            <Markdown source={c.body} />
          </li>
        ))}
      </ol>
      <form
        className="projects-comment-form"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <MarkdownEditor
          polish="comment"
          uploadScope="projects"
          label={t("Comment")}
          value={body}
          onChange={setBody}
          minRows={3}
          placeholder={t("Write a progress note…")}
          onSubmit={submit}
        />
        <div className="xc-row">
          <span className="xc-muted projects-hint">⌘/Ctrl + Enter</span>
          <span className="xc-spacer" />
          <button
            className="xc-btn small primary"
            disabled={!body.trim() || add.isPending}
          >
            {t("Comment")}
          </button>
        </div>
      </form>
    </>
  );
  if (bare) return list;
  return (
    <section className="projects-section">
      <header>
        <h2>{t("Comments")}</h2>
      </header>
      {list}
    </section>
  );
}

const ACTIVITY_TEXT: Record<string, string> = {
  created: "created this card",
  moved: "moved it to",
  archived: "archived it",
  restored: "restored it",
  members: "changed members",
  copied: "copied it from",
  color: "changed the color",
};

/** 活动记录（B46）：谁在什么时候做了什么。 */
function Activity({
  issueKey,
  bare,
}: {
  issueKey: string;
  bare?: boolean;
}) {
  const t = useT();
  const language = useLanguage();
  const activity = useIssueActivity(issueKey);
  const items = activity.data ?? [];
  if (!items.length) {
    if (!bare) return null;
    return <p className="xc-muted projects-section-empty">{t("No activity yet")}</p>;
  }
  const who = (actor: string) =>
    actor === "me" || !actor.includes(":") ? t("Me") : actor;
  return (
    <section className="projects-activity">
      <h2>{t("Activity")}</h2>
      <ol>
        {items.map((a) => (
          <li key={a.id}>
            <strong>{who(a.actor)}</strong> {t(ACTIVITY_TEXT[a.kind] ?? a.kind)}
            {a.kind === "moved" && typeof a.data.toList === "string"
              ? ` “${a.data.toList}”`
              : ""}
            {a.kind === "copied" && typeof a.data.from === "string"
              ? ` ${a.data.from}`
              : ""}
            <time dateTime={a.at} title={a.at}>
              {relativeTime(a.at, language)}
            </time>
          </li>
        ))}
      </ol>
    </section>
  );
}

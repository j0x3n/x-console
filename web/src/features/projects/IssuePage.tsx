import { useEffect, useState } from "react";
import MarkdownEditor from "../../components/markdown/MarkdownEditor";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { usePageCrumb } from "../../stores/page-title";
import {
  Archive,
  ArchiveRestore,
  Bot,
  Copy,
  ExternalLink,
  GitPullRequest,
  Link2,
  NotebookPen,
  Plus,
  Trash2,
} from "lucide-react";
import { ApiError } from "../../api/client";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
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
import Checklists from "./components/Checklists";
import DueFields from "./components/DueFields";
import Markdown from "./Markdown";
import StartFocusButton from "../calendar/StartFocusButton";
import AssignDialog from "../aiagents/AssignDialog";
import AgentMember from "../aiagents/AgentMember";
import {
  PRIORITIES,
  PRIORITY_LABELS,
  STATUSES,
  STATUS_LABELS,
  hasMember,
  issuePath,
  joinDue,
  splitDue,
  toggleMember,
  type IssueStatus,
} from "./logic";
import { useShortcuts } from "./useShortcuts";
import { confirmAction } from "../../components/ui/ConfirmDialog";

export default function IssuePage() {
  const t = useT();
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
  const update = useUpdateIssue();
  const [search, setSearch] = useSearchParams();
  const [editingTitle, setEditingTitle] = useState(
    search.get("edit") === "title",
  );
  const [addingChecklist, setAddingChecklist] = useState(false);

  useEffect(() => {
    if (search.get("edit") === "title") {
      setEditingTitle(true);
      search.delete("edit");
      setSearch(search, { replace: true });
    }
  }, [search, setSearch]);

  const save = (body: UpdateIssue) =>
    issue.data &&
    update.mutate({ issue: issue.data, key: issue.data.key, body });

  useShortcuts(
    {
      e: () => setEditingTitle(true),
      escape: () => navigate(`/projects/${projectKey.toUpperCase()}`),
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

  if (issue.isPending) return <Loading />;
  if (issue.isError) {
    return (
      <div className="xc-page">
        {issue.error instanceof ApiError && issue.error.status === 404 ? (
          <EmptyState title={t("Issue not found")}>
            <Link to={`/projects/${projectKey.toUpperCase()}`}>
              {t("Back to project")}
            </Link>
          </EmptyState>
        ) : (
          <ErrorState error={issue.error} onRetry={() => issue.refetch()} />
        )}
      </div>
    );
  }
  const data = issue.data;
  return (
    <div className="xc-page wide projects-issue-page">
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
          <Comments issueKey={data.key} />
          <Activity issueKey={data.key} />
        </main>
        <aside className="projects-issue-side">
          <Properties issue={data} onSave={save} />
        </aside>
      </div>
    </div>
  );
}

function IssueTitle({
  issue,
  editing,
  setEditing,
  onSave,
}: {
  issue: Issue;
  editing: boolean;
  setEditing: (v: boolean) => void;
  onSave: (title: string) => void;
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
        className="xc-input projects-title-input"
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
      className="projects-issue-title"
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
}: {
  issue: Issue;
  onSave: (description: string) => void;
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
  if (!editing)
    return (
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
    );
  return (
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
        uploadScope="projects"
        label={t("Description")}
        value={value}
        onChange={setValue}
        autoFocus
        minRows={10}
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
}

function Properties({
  issue,
  onSave,
}: {
  issue: Issue;
  onSave: (body: UpdateIssue) => void;
}) {
  const t = useT();
  const language = useLanguage();
  const navigate = useNavigate();
  const labels = useLabels(issue.projectId);
  const milestones = useMilestones(issue.projectId);
  const { live } = useB36Live(issue.projectId);
  const boards = useBoards(issue.projectId);
  const move = useMoveCard();
  const cards = useCardActions();
  const remove = useDeleteIssue();
  const [assigning, setAssigning] = useState(false);
  const labelIds = issue.labels.map((l) => l.id);
  return (
    <div className="projects-props">
      <label className="projects-prop">
        <span>{t("Status")}</span>
        <div className="projects-prop-control">
          <StatusIcon status={issue.status} />
          <select
            className="xc-select"
            value={issue.status}
            onChange={(e) => onSave({ status: e.target.value as IssueStatus })}
          >
            {STATUSES.map((s, i) => (
              <option key={s} value={s}>
                {t(STATUS_LABELS[s])} ({i + 1})
              </option>
            ))}
          </select>
        </div>
      </label>
      <label className="projects-prop">
        <span>{t("Priority")}</span>
        <div className="projects-prop-control">
          <PriorityIcon priority={issue.priority} />
          <select
            className="xc-select"
            value={issue.priority}
            onChange={(e) => onSave({ priority: Number(e.target.value) })}
          >
            {PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {t(PRIORITY_LABELS[p])}
              </option>
            ))}
          </select>
        </div>
      </label>
      {boards.data && issue.listId !== undefined && (
        <label className="projects-prop">
          <span>{t("Board and list")}</span>
          <ListSelect
            label={t("Board and list")}
            boards={boards.data}
            value={issue.listId}
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
        <select
          className="xc-select"
          value={issue.milestoneId ?? ""}
          onChange={(e) =>
            onSave({
              milestoneId: e.target.value ? Number(e.target.value) : null,
            })
          }
        >
          <option value="">{t("None")}</option>
          {milestones.data?.map((m) => (
            <option key={m.id} value={m.id}>
              {m.name}
            </option>
          ))}
        </select>
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
      <dl className="projects-meta">
        <dt>{t("Created")}</dt>
        <dd title={issue.createdAt}>
          {relativeTime(issue.createdAt, language)}
        </dd>
        <dt>{t("Updated")}</dt>
        <dd title={issue.updatedAt}>
          {relativeTime(issue.updatedAt, language)}
        </dd>
        {issue.completedAt && (
          <>
            <dt>{t("Completed")}</dt>
            <dd title={issue.completedAt}>
              {relativeTime(issue.completedAt, language)}
            </dd>
          </>
        )}
      </dl>
      <div className="projects-coding">
        <button className="xc-btn" onClick={() => setAssigning(true)}>
          <Bot size={14} /> {t("Assign to an agent")}
        </button>
        <Link
          className="xc-btn ghost small"
          to={`/coding/tasks?new=1&issue=${encodeURIComponent(issue.key)}`}
        >
          {t("New coding task by hand")}
        </Link>
      </div>
      {assigning && (
        <AssignDialog
          issueKey={issue.key}
          onClose={() => setAssigning(false)}
        />
      )}
      <StartFocusButton issueKey={issue.key} />
      <div className="projects-card-actions">
        <button
          className="xc-btn small"
          disabled={cards.copy.isPending}
          onClick={() =>
            cards.copy.mutate(issue.key, {
              onSuccess: (copy) => navigate(issuePath(copy.key)),
            })
          }
        >
          <Copy size={14} /> {t("Copy card")}
        </button>
        {issue.archivedAt ? (
          <button
            className="xc-btn small"
            disabled={cards.restore.isPending}
            onClick={() => cards.restore.mutate(issue.key)}
          >
            <ArchiveRestore size={14} /> {t("Restore")}
          </button>
        ) : (
          <button
            className="xc-btn small"
            disabled={cards.archive.isPending}
            onClick={() => cards.archive.mutate(issue.key)}
          >
            <Archive size={14} /> {t("Archive")}
          </button>
        )}
      </div>
      <button
        className="xc-btn danger small"
        disabled={remove.isPending}
        onClick={async () => {
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
        }}
      >
        <Trash2 size={14} /> {t("Delete issue")}
      </button>
    </div>
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

function Comments({ issueKey }: { issueKey: string }) {
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
  return (
    <section className="projects-section">
      <header>
        <h2>{t("Comments")}</h2>
      </header>
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
};

/** 活动记录（B46）：谁在什么时候做了什么。 */
function Activity({ issueKey }: { issueKey: string }) {
  const t = useT();
  const language = useLanguage();
  const activity = useIssueActivity(issueKey);
  const items = activity.data ?? [];
  if (!items.length) return null;
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

import { useEffect, useState } from "react";
import MarkdownEditor from "../../../components/markdown/MarkdownEditor";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  useCreateIssue,
  useLabels,
  useMilestones,
  useProjects,
  type Issue,
} from "../api";
import {
  PRIORITIES,
  PRIORITY_LABELS,
  STATUSES,
  STATUS_LABELS,
  type IssueStatus,
} from "../logic";
import { LabelChip } from "./Icons";

const LAST_PROJECT = "projects.lastProject";

export function rememberProject(id: number) {
  try {
    localStorage.setItem(LAST_PROJECT, String(id));
  } catch {
    /* 浏览器禁止存储时忽略 */
  }
}

function lastProject(): number | undefined {
  try {
    const v = Number(localStorage.getItem(LAST_PROJECT));
    return Number.isFinite(v) && v > 0 ? v : undefined;
  } catch {
    return undefined;
  }
}

/** 新建 Issue 的弹窗。Ctrl/⌘ + Enter 提交。 */
export default function NewIssueDialog({
  open,
  onClose,
  projectId,
  status = "todo",
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  projectId?: number;
  status?: IssueStatus;
  onCreated?: (issue: Issue) => void;
}) {
  const t = useT();
  const projects = useProjects();
  const [pid, setPid] = useState<number | undefined>(projectId);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [issueStatus, setIssueStatus] = useState<IssueStatus>(status);
  const [priority, setPriority] = useState(0);
  const [dueDate, setDueDate] = useState("");
  const [milestoneId, setMilestoneId] = useState<number | null>(null);
  const [labelIds, setLabelIds] = useState<number[]>([]);
  const [error, setError] = useState("");
  const labels = useLabels(pid);
  const milestones = useMilestones(pid);
  const create = useCreateIssue();

  useEffect(() => {
    if (!open) return;
    setTitle("");
    setDescription("");
    setIssueStatus(status);
    setPriority(0);
    setDueDate("");
    setMilestoneId(null);
    setLabelIds([]);
    setError("");
    setPid(projectId);
  }, [open, projectId, status]);

  // 没指定项目时，用上次的项目或第一个项目。
  useEffect(() => {
    if (!open || pid !== undefined || !projects.data?.length) return;
    const last = lastProject();
    setPid(
      projects.data.find((p) => p.id === last)?.id ?? projects.data[0].id,
    );
  }, [open, pid, projects.data]);

  const submit = () => {
    if (!pid || !title.trim()) return;
    setError("");
    create.mutate(
      {
        projectId: pid,
        body: {
          title: title.trim(),
          description,
          status: issueStatus,
          priority,
          dueDate: dueDate || undefined,
          milestoneId: milestoneId ?? undefined,
          labelIds,
        },
      },
      {
        onSuccess: (issue) => {
          rememberProject(pid);
          toast(`${t("Created")} ${issue.key}`);
          onClose();
          onCreated?.(issue);
        },
        onError: (err) => setError(errorMessage(err)),
      },
    );
  };

  return (
    <Dialog open={open} onClose={onClose} title={t("New issue")} wide>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            submit();
          }
        }}
      >
        {projectId === undefined && (
          <label className="xc-field">
            <span>{t("Project")}</span>
            <select
              className="xc-select"
              value={pid ?? ""}
              onChange={(e) => {
                setPid(Number(e.target.value));
                setLabelIds([]);
                setMilestoneId(null);
              }}
            >
              {projects.data?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.key} · {p.name}
                </option>
              ))}
            </select>
          </label>
        )}
        <label className="xc-field">
          <span>{t("Title")}</span>
          <input
            className="xc-input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={t("What needs to be done?")}
            autoFocus
            required
          />
        </label>
        <div className="xc-field">
          <span>{t("Description")}</span>
          <MarkdownEditor
            label={t("Description")}
            value={description}
            onChange={setDescription}
            placeholder={t("Markdown is supported")}
            minRows={5}
          />
        </div>
        <div className="projects-form-grid">
          <label className="xc-field">
            <span>{t("Status")}</span>
            <select
              className="xc-select"
              value={issueStatus}
              onChange={(e) => setIssueStatus(e.target.value as IssueStatus)}
            >
              {STATUSES.map((s) => (
                <option key={s} value={s}>
                  {t(STATUS_LABELS[s])}
                </option>
              ))}
            </select>
          </label>
          <label className="xc-field">
            <span>{t("Priority")}</span>
            <select
              className="xc-select"
              value={priority}
              onChange={(e) => setPriority(Number(e.target.value))}
            >
              {PRIORITIES.map((p) => (
                <option key={p} value={p}>
                  {t(PRIORITY_LABELS[p])}
                </option>
              ))}
            </select>
          </label>
          <label className="xc-field">
            <span>{t("Due date")}</span>
            <input
              className="xc-input"
              type="date"
              value={dueDate}
              onChange={(e) => setDueDate(e.target.value)}
            />
          </label>
          <label className="xc-field">
            <span>{t("Milestone")}</span>
            <select
              className="xc-select"
              value={milestoneId ?? ""}
              onChange={(e) =>
                setMilestoneId(e.target.value ? Number(e.target.value) : null)
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
        </div>
        {!!labels.data?.length && (
          <div className="xc-field">
            <span>{t("Labels")}</span>
            <div className="projects-label-picker">
              {labels.data.map((label) => (
                <button
                  type="button"
                  key={label.id}
                  className={labelIds.includes(label.id) ? "on" : ""}
                  aria-pressed={labelIds.includes(label.id)}
                  onClick={() =>
                    setLabelIds((ids) =>
                      ids.includes(label.id)
                        ? ids.filter((id) => id !== label.id)
                        : [...ids, label.id],
                    )
                  }
                >
                  <LabelChip label={label} />
                </button>
              ))}
            </div>
          </div>
        )}
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <span className="xc-muted projects-hint">⌘/Ctrl + Enter</span>
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={!pid || !title.trim() || create.isPending}
          >
            {t("Create issue")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

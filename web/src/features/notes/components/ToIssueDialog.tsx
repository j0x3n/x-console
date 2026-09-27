import { useEffect, useState } from "react";
import { Link } from "react-router";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useProjects } from "../../projects/api";
import { useNoteToIssue, type Note } from "../api";

/** 用笔记建 Issue：选项目，建好后笔记末尾会多一行 Issue 链接。 */
export default function ToIssueDialog({
  open,
  onClose,
  noteId,
  onDone,
}: {
  open: boolean;
  onClose: () => void;
  noteId: number;
  onDone: (note: Note) => void;
}) {
  const t = useT();
  const projects = useProjects();
  const toIssue = useNoteToIssue();
  const [projectId, setProjectId] = useState<number | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open && projectId === null && projects.data?.length)
      setProjectId(projects.data[0].id);
  }, [open, projectId, projects.data]);
  useEffect(() => {
    if (open) setError("");
  }, [open]);

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("Turn into issue")}
      description={t(
        "The note becomes the issue description. A link to the issue is added to the note.",
      )}
    >
      {projects.data?.length === 0 ? (
        <p className="xc-muted">
          {t("No projects yet")} ·{" "}
          <Link to="/projects?newProject=1">{t("New project")}</Link>
        </p>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (!projectId) return;
            toIssue.mutate(
              { id: noteId, projectId },
              {
                onSuccess: (out) => {
                  onDone(out.note);
                  toast(`${t("Created")} ${out.issueKey}`);
                  onClose();
                },
                onError: (err) => setError(errorMessage(err)),
              },
            );
          }}
        >
          <label className="xc-field">
            <span>{t("Project")}</span>
            <select
              className="xc-select"
              value={projectId ?? ""}
              onChange={(e) => setProjectId(Number(e.target.value))}
            >
              {projects.data?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.key} · {p.name}
                </option>
              ))}
            </select>
          </label>
          {error && <p className="xc-error-text">{error}</p>}
          <div className="xc-dialog-actions">
            <button type="button" className="xc-btn" onClick={onClose}>
              {t("Cancel")}
            </button>
            <button
              className="xc-btn primary"
              disabled={!projectId || toIssue.isPending}
            >
              {t("Create issue")}
            </button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

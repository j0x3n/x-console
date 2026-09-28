import { useState } from "react";
import { Archive, ArchiveRestore, Plus, Trash2 } from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import {
  useLabelMutations,
  useLabels,
  useMilestoneMutations,
  useMilestones,
  useUpdateProject,
  type Project,
} from "../api";
import { LabelChip, PROJECT_COLORS } from "./Icons";
import { confirmAction } from "../../../components/ui/ConfirmDialog";

/** 管理项目的标签、里程碑，以及归档。 */
export default function ProjectSettingsDialog({
  open,
  onClose,
  project,
}: {
  open: boolean;
  onClose: () => void;
  project: Project;
}) {
  const t = useT();
  const labels = useLabels(project.id);
  const milestones = useMilestones(project.id);
  const labelOps = useLabelMutations(project.id);
  const milestoneOps = useMilestoneMutations(project.id);
  const updateProject = useUpdateProject();
  const [labelName, setLabelName] = useState("");
  const [labelColor, setLabelColor] = useState(PROJECT_COLORS[4]);
  const [labelGlobal, setLabelGlobal] = useState(false);
  const [msName, setMsName] = useState("");
  const [msDue, setMsDue] = useState("");
  const archived = !!project.archivedAt;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={`${project.name} · ${t("Settings")}`}
      wide
    >
      <section className="projects-settings-section">
        <h3>{t("Labels")}</h3>
        <ul className="projects-settings-list">
          {labels.data?.map((label) => (
            <li key={label.id}>
              <LabelChip label={label} />
              {label.projectId === undefined && (
                <span className="xc-badge">{t("All projects")}</span>
              )}
              <span className="xc-spacer" />
              <button
                className="xc-btn ghost small"
                aria-label={t("Delete")}
                onClick={async () =>
                  (await confirmAction({
                    title: `${t("Delete label")}“${label.name}”？`,
                    description: t("It is removed from every issue."),
                  })) && labelOps.remove.mutate(label.id)
                }
              >
                <Trash2 size={14} />
              </button>
            </li>
          ))}
          {labels.data?.length === 0 && (
            <li className="xc-muted">{t("No labels yet")}</li>
          )}
        </ul>
        <form
          className="projects-inline-form"
          onSubmit={(e) => {
            e.preventDefault();
            if (!labelName.trim()) return;
            labelOps.create.mutate(
              {
                name: labelName.trim(),
                color: labelColor,
                global: labelGlobal,
              },
              { onSuccess: () => setLabelName("") },
            );
          }}
        >
          <input
            className="xc-input"
            value={labelName}
            onChange={(e) => setLabelName(e.target.value)}
            placeholder={t("Label name")}
          />
          <select
            className="xc-select"
            value={labelColor}
            onChange={(e) => setLabelColor(e.target.value)}
            aria-label={t("Color")}
            style={{ color: labelColor }}
          >
            {PROJECT_COLORS.map((c) => (
              <option key={c} value={c} style={{ color: c }}>
                ● {c}
              </option>
            ))}
          </select>
          <label className="projects-check">
            <input
              type="checkbox"
              checked={labelGlobal}
              onChange={(e) => setLabelGlobal(e.target.checked)}
            />
            {t("All projects")}
          </label>
          <button className="xc-btn small" disabled={!labelName.trim()}>
            <Plus size={14} /> {t("Add")}
          </button>
        </form>
      </section>

      <section className="projects-settings-section">
        <h3>{t("Milestones")}</h3>
        <ul className="projects-settings-list">
          {milestones.data?.map((ms) => (
            <li key={ms.id}>
              <strong>{ms.name}</strong>
              <input
                className="xc-input projects-date-input"
                type="date"
                value={ms.dueDate ?? ""}
                aria-label={t("Due date")}
                onChange={(e) =>
                  milestoneOps.update.mutate({
                    id: ms.id,
                    body: { dueDate: e.target.value || null },
                  })
                }
              />
              <span className="xc-spacer" />
              <button
                className="xc-btn ghost small"
                aria-label={t("Delete")}
                onClick={async () =>
                  (await confirmAction({
                    title: `${t("Delete milestone")}“${ms.name}”？`,
                  })) && milestoneOps.remove.mutate(ms.id)
                }
              >
                <Trash2 size={14} />
              </button>
            </li>
          ))}
          {milestones.data?.length === 0 && (
            <li className="xc-muted">{t("No milestones yet")}</li>
          )}
        </ul>
        <form
          className="projects-inline-form"
          onSubmit={(e) => {
            e.preventDefault();
            if (!msName.trim()) return;
            milestoneOps.create.mutate(
              { name: msName.trim(), dueDate: msDue || undefined },
              {
                onSuccess: () => {
                  setMsName("");
                  setMsDue("");
                },
              },
            );
          }}
        >
          <input
            className="xc-input"
            value={msName}
            onChange={(e) => setMsName(e.target.value)}
            placeholder={t("Milestone name")}
          />
          <input
            className="xc-input projects-date-input"
            type="date"
            value={msDue}
            onChange={(e) => setMsDue(e.target.value)}
            aria-label={t("Due date")}
          />
          <button className="xc-btn small" disabled={!msName.trim()}>
            <Plus size={14} /> {t("Add")}
          </button>
        </form>
      </section>

      <div className="xc-dialog-actions">
        <button
          className={`xc-btn ${archived ? "" : "danger"}`}
          disabled={updateProject.isPending}
          onClick={async () => {
            if (
              !archived &&
              !(await confirmAction({
                title: t("Archive this project? You can restore it later."),
                confirmLabel: t("Archive project"),
              }))
            )
              return;
            updateProject.mutate(
              { id: project.id, body: { archived: !archived } },
              { onSuccess: onClose },
            );
          }}
        >
          {archived ? <ArchiveRestore size={14} /> : <Archive size={14} />}
          {archived ? t("Restore project") : t("Archive project")}
        </button>
        <span className="xc-spacer" />
        <button className="xc-btn" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

import { useState } from "react";
import {
  Archive,
  ArchiveRestore,
  ArrowDown,
  ArrowUp,
  Plus,
  Trash2,
} from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import {
  useCategories,
  useCategoryMutations,
  useLabelMutations,
  useLabels,
  useMilestoneMutations,
  useMilestones,
  useUpdateProject,
  type Project,
} from "../api";
import { LabelChip, PROJECT_COLORS } from "./Icons";
import { confirmAction } from "../../../components/ui/ConfirmDialog";
import { isNotLive } from "../../../api/client";
import {
  categoryTree,
  planCategoryStep,
  type Category,
  type CategoryNode,
} from "../logic";

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
      {/* B46：分类换成了多看板，分类管理不再显示（CategoriesSection 保留到下个版本删）。 */}

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

/** 分类（B36）：最多两级，可以改名、上下移动、删除。 */
function CategoriesSection({ projectId }: { projectId: number }) {
  const t = useT();
  const categories = useCategories(projectId);
  const ops = useCategoryMutations(projectId);
  const [name, setName] = useState("");
  const [parentId, setParentId] = useState<number | null>(null);
  const list = Array.isArray(categories.data) ? categories.data : [];
  const tree = categoryTree(list);
  const parents = tree.filter((n) => n.depth === 0);

  if (categories.isError && isNotLive(categories.error))
    return (
      <section className="projects-settings-section">
        <h3>{t("Categories")}</h3>
        <p className="xc-muted">{t("Categories are not live yet.")}</p>
      </section>
    );

  const step = (c: Category, delta: 1 | -1) => {
    const plan = planCategoryStep(list, c.id, delta);
    if (plan) ops.update.mutate({ id: c.id, body: plan });
  };

  return (
    <section className="projects-settings-section">
      <h3>{t("Categories")}</h3>
      <ul className="projects-settings-list">
        {tree.map((node) => (
          <CategoryRow
            key={node.category.id}
            node={node}
            canUp={!!planCategoryStep(list, node.category.id, -1)}
            canDown={!!planCategoryStep(list, node.category.id, 1)}
            onStep={(delta) => step(node.category, delta)}
            onRename={(value) =>
              ops.update.mutate({ id: node.category.id, body: { name: value } })
            }
            onAddChild={() => setParentId(node.category.id)}
            onDelete={async () =>
              (await confirmAction({
                title: `${t("Delete category")}“${node.path}”？`,
                description:
                  node.depth === 0 &&
                  list.some((c) => c.parentId === node.category.id)
                    ? t(
                        "Its subcategories are deleted too. Their issues become uncategorized.",
                      )
                    : t("Its issues become uncategorized."),
              })) && ops.remove.mutate(node.category.id)
            }
          />
        ))}
        {categories.isSuccess && tree.length === 0 && (
          <li className="xc-muted">
            {t("No categories yet. Group issues like Backend / Drive.")}
          </li>
        )}
      </ul>
      <form
        className="projects-inline-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (!name.trim()) return;
          ops.create.mutate(
            { name: name.trim(), parentId: parentId ?? undefined },
            { onSuccess: () => setName("") },
          );
        }}
      >
        <input
          className="xc-input"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t("Category name")}
          maxLength={40}
        />
        <select
          className="xc-select"
          value={parentId ?? ""}
          aria-label={t("Parent category")}
          onChange={(e) =>
            setParentId(e.target.value ? Number(e.target.value) : null)
          }
        >
          <option value="">{t("Top level")}</option>
          {parents.map((p) => (
            <option key={p.category.id} value={p.category.id}>
              {t("Under")} {p.category.name}
            </option>
          ))}
        </select>
        <button
          className="xc-btn small"
          disabled={!name.trim() || ops.create.isPending}
        >
          <Plus size={14} /> {t("Add")}
        </button>
      </form>
    </section>
  );
}

function CategoryRow({
  node,
  canUp,
  canDown,
  onStep,
  onRename,
  onAddChild,
  onDelete,
}: {
  node: CategoryNode;
  canUp: boolean;
  canDown: boolean;
  onStep: (delta: 1 | -1) => void;
  onRename: (name: string) => void;
  onAddChild: () => void;
  onDelete: () => void;
}) {
  const t = useT();
  const c = node.category;
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(c.name);
  const commit = () => {
    setEditing(false);
    const name = value.trim();
    if (name && name !== c.name) onRename(name);
    else setValue(c.name);
  };
  return (
    <li className={node.depth ? "projects-category-child" : undefined}>
      {editing ? (
        <input
          className="xc-input"
          value={value}
          autoFocus
          maxLength={40}
          aria-label={t("Category name")}
          onChange={(e) => setValue(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") commit();
            if (e.key === "Escape") {
              e.stopPropagation();
              setValue(c.name);
              setEditing(false);
            }
          }}
        />
      ) : (
        <button
          type="button"
          className="projects-category-name"
          title={t("Click to rename")}
          onClick={() => {
            setValue(c.name);
            setEditing(true);
          }}
        >
          {c.name}
        </button>
      )}
      <span className="xc-muted">{c.issueCount}</span>
      <span className="xc-spacer" />
      {node.depth === 0 && (
        <button
          className="xc-btn ghost small"
          title={t("Add subcategory")}
          aria-label={`${t("Add subcategory")}: ${c.name}`}
          onClick={onAddChild}
        >
          <Plus size={14} />
        </button>
      )}
      <button
        className="xc-btn ghost small"
        aria-label={`${t("Move up")}: ${c.name}`}
        disabled={!canUp}
        onClick={() => onStep(-1)}
      >
        <ArrowUp size={14} />
      </button>
      <button
        className="xc-btn ghost small"
        aria-label={`${t("Move down")}: ${c.name}`}
        disabled={!canDown}
        onClick={() => onStep(1)}
      >
        <ArrowDown size={14} />
      </button>
      <button
        className="xc-btn ghost small"
        aria-label={`${t("Delete")}: ${c.name}`}
        onClick={onDelete}
      >
        <Trash2 size={14} />
      </button>
    </li>
  );
}

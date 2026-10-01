import { useEffect, useState } from "react";
import MarkdownEditor from "../../../components/markdown/MarkdownEditor";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useCreateProject, useUpdateProject, type Project } from "../api";
import { PROJECT_COLORS } from "./Icons";

/** 从名称猜一个 key：取英文首字母，不够就用 PRJ。 */
export function suggestKey(name: string): string {
  const words = name.toUpperCase().match(/[A-Z]+/g) ?? [];
  let key =
    words.length > 1
      ? words.map((w) => w[0]).join("")
      : (words[0] ?? "").slice(0, 3);
  key = key.slice(0, 5);
  return key.length >= 2 ? key : "PRJ";
}

/** 新建或编辑项目。 */
export default function ProjectDialog({
  open,
  onClose,
  project,
  onSaved,
}: {
  open: boolean;
  onClose: () => void;
  project?: Project;
  onSaved?: (project: Project) => void;
}) {
  const t = useT();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [keyTouched, setKeyTouched] = useState(false);
  const [description, setDescription] = useState("");
  const [color, setColor] = useState(PROJECT_COLORS[0]);
  const [error, setError] = useState("");
  const create = useCreateProject();
  const update = useUpdateProject();

  useEffect(() => {
    if (!open) return;
    setName(project?.name ?? "");
    setKey(project?.key ?? "");
    setKeyTouched(!!project);
    setDescription(project?.description ?? "");
    setColor(project?.color || PROJECT_COLORS[0]);
    setError("");
  }, [open, project]);

  const done = (saved: Project) => {
    toast(t("Saved"));
    onClose();
    onSaved?.(saved);
  };
  const submit = () => {
    setError("");
    if (project)
      update.mutate(
        { id: project.id, body: { name, description, color } },
        { onSuccess: done, onError: (err) => setError(errorMessage(err)) },
      );
    else
      create.mutate(
        { key, name, description, color },
        { onSuccess: done, onError: (err) => setError(errorMessage(err)) },
      );
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={project ? t("Edit project") : t("New project")}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              if (!keyTouched) setKey(suggestKey(e.target.value));
            }}
            autoFocus
            required
          />
        </label>
        {!project && (
          <label className="xc-field">
            <span>Key</span>
            <input
              className="xc-input xc-mono"
              value={key}
              onChange={(e) => {
                setKeyTouched(true);
                setKey(
                  e.target.value
                    .toUpperCase()
                    .replace(/[^A-Z]/g, "")
                    .slice(0, 5),
                );
              }}
              pattern="[A-Z]{2,5}"
              required
            />
            <small>
              {t(
                "2 to 5 capital letters. Issue keys look like XC-12. It cannot be changed later.",
              )}
            </small>
          </label>
        )}
        <div className="xc-field">
          <span>{t("Description")}</span>
          <MarkdownEditor
            polish="card"
            uploadScope="projects"
            label={t("Description")}
            value={description}
            onChange={setDescription}
            minRows={4}
          />
        </div>
        <div className="xc-field">
          <span>{t("Color")}</span>
          <div className="projects-colors">
            {PROJECT_COLORS.map((c) => (
              <button
                type="button"
                key={c}
                style={{ background: c }}
                className={c === color ? "on" : ""}
                aria-label={c}
                aria-pressed={c === color}
                onClick={() => setColor(c)}
              />
            ))}
          </div>
        </div>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={
              !name.trim() ||
              (!project && key.length < 2) ||
              create.isPending ||
              update.isPending
            }
          >
            {project ? t("Save") : t("Create project")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

import { useState } from "react";
import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import Dialog from "../../../components/ui/Dialog";
import { Segmented } from "../../../components/ui/Toolbar";
import { errorMessage } from "../../../api/client";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useSetBuildConfig, type BuildStep, type Repo } from "../api";

type OS = "linux" | "windows";

/** B47：仓库的构建步骤，Linux 和 Windows 各一套。 */
export default function BuildConfigDialog({
  repo,
  onClose,
}: {
  repo: Repo;
  onClose: () => void;
}) {
  const t = useT();
  const save = useSetBuildConfig();
  const [os, setOS] = useState<OS>("linux");
  const [steps, setSteps] = useState<Record<OS, BuildStep[]>>({
    linux: repo.buildConfig?.linux ?? [],
    windows: repo.buildConfig?.windows ?? [],
  });
  const [error, setError] = useState("");
  const list = steps[os];
  const set = (next: BuildStep[]) => setSteps({ ...steps, [os]: next });
  const patch = (i: number, p: Partial<BuildStep>) =>
    set(list.map((s, j) => (j === i ? { ...s, ...p } : s)));
  const move = (i: number, d: number) => {
    const next = [...list];
    [next[i], next[i + d]] = [next[i + d], next[i]];
    set(next);
  };
  return (
    <Dialog
      open
      onClose={onClose}
      title={`${t("Build steps")}：${repo.remoteRepo ?? repo.name}`}
      wide
    >
      <p className="xc-muted">
        {t(
          "Run in order in the task's work folder after the agent's change. A failed step stops the build. The machine's system picks the set.",
        )}
      </p>
      <Segmented
        label={t("System")}
        value={os}
        onChange={setOS}
        options={[
          { value: "linux", label: `Linux (${steps.linux.length})` },
          { value: "windows", label: `Windows (${steps.windows.length})` },
        ]}
      />
      <ol className="coding-steps">
        {list.map((s, i) => (
          <li key={i}>
            <div className="coding-step-row">
              <input
                className="xc-input"
                aria-label={t("Step name")}
                placeholder={t("Step name")}
                value={s.name}
                maxLength={60}
                onChange={(e) => patch(i, { name: e.target.value })}
              />
              <button
                type="button"
                className="xc-btn ghost small"
                disabled={i === 0}
                aria-label={t("Move up")}
                onClick={() => move(i, -1)}
              >
                <ArrowUp size={14} />
              </button>
              <button
                type="button"
                className="xc-btn ghost small"
                disabled={i === list.length - 1}
                aria-label={t("Move down")}
                onClick={() => move(i, 1)}
              >
                <ArrowDown size={14} />
              </button>
              <button
                type="button"
                className="xc-btn ghost small"
                aria-label={t("Delete")}
                onClick={() => set(list.filter((_, j) => j !== i))}
              >
                <Trash2 size={14} />
              </button>
            </div>
            <input
              className="xc-input xc-mono"
              aria-label={t("Command")}
              placeholder={
                os === "linux" ? "go test ./..." : "dotnet publish -c Release"
              }
              value={s.command}
              onChange={(e) => patch(i, { command: e.target.value })}
            />
            <div className="coding-step-row">
              <input
                className="xc-input xc-mono"
                aria-label={t("Artifacts")}
                placeholder={t(
                  "Artifacts, for example dist/** or *.zip, separated by commas",
                )}
                value={(s.artifacts ?? []).join(", ")}
                onChange={(e) =>
                  patch(i, {
                    artifacts: e.target.value
                      .split(",")
                      .map((x) => x.trim())
                      .filter(Boolean),
                  })
                }
              />
              <input
                className="xc-input coding-step-timeout"
                type="number"
                min={0}
                aria-label={t("Timeout in minutes")}
                title={t("Timeout in minutes")}
                placeholder="30"
                value={
                  s.timeoutSeconds ? Math.round(s.timeoutSeconds / 60) : ""
                }
                onChange={(e) =>
                  patch(i, {
                    timeoutSeconds: e.target.value
                      ? Number(e.target.value) * 60
                      : undefined,
                  })
                }
              />
            </div>
          </li>
        ))}
      </ol>
      <button
        type="button"
        className="xc-btn small"
        disabled={list.length >= 20}
        onClick={() => set([...list, { name: "", command: "" }])}
      >
        <Plus size={14} /> {t("Add step")}
      </button>
      {error && <p className="xc-error-text">{error}</p>}
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button
          type="button"
          className="xc-btn primary"
          disabled={save.isPending}
          onClick={async () => {
            setError("");
            try {
              await save.mutateAsync({ id: repo.id, config: steps });
              toast(t("Saved"));
              onClose();
            } catch (err) {
              setError(errorMessage(err));
            }
          }}
        >
          {t("Save")}
        </button>
      </div>
    </Dialog>
  );
}

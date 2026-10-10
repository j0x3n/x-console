import { useState } from "react";
import { Save } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import PageHeading from "../../components/ui/PageHeading";
import { ErrorState, Loading, NotLive } from "../../components/ui/States";
import { toast } from "../../hooks/useToast";
import { useT } from "../../contexts/LanguageContext";
import {
  useAIConfig,
  useAIConfigStatus,
  useApplyAIConfig,
  useSaveAIConfig,
  type AIConfigHostStatus,
} from "./api";
import HostsCard from "./HostsCard";
import ToolEditor from "./ToolEditor";
import {
  problemCount,
  sameAsSaved,
  toDraft,
  toInput,
  type Draft,
} from "./format";
import "./i18n";
import "./aiconfig.css";

const showError = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 配置下发（B121）：一份 Claude Code 和 Codex 配置，写到所选机器，显示哪台不一致。 */
export default function AIConfigPage() {
  const t = useT();
  const cfg = useAIConfig();
  const save = useSaveAIConfig();
  const apply = useApplyAIConfig();
  const saved = cfg.data;
  const [edits, setEdits] = useState<Draft | null>(null);
  const draft = edits ?? (saved ? toDraft(saved) : null);
  const dirty = !!(edits && saved && !sameAsSaved(edits, saved));
  const status = useAIConfigStatus(!!saved && saved.hostIds.length > 0);

  if (cfg.isPending) return <Loading />;
  if (cfg.isError) {
    if (isNotLive(cfg.error)) return <NotLive name={t("Config delivery")} />;
    return <ErrorState error={cfg.error} onRetry={() => cfg.refetch()} />;
  }
  if (!draft || !saved) return <Loading />;

  const change = (next: Draft) => setEdits(next);
  const toggleHost = (id: string, on: boolean) =>
    change({
      ...draft,
      hostIds: on
        ? [...draft.hostIds.filter((x) => x !== id), id]
        : draft.hostIds.filter((x) => x !== id),
    });

  const onSave = () =>
    save.mutate(toInput(draft), {
      onSuccess: () => {
        setEdits(null);
        toast(t("Saved"));
      },
      onError: showError,
    });

  const onApply = async (hostId?: string) => {
    const count = hostId ? 1 : saved.hostIds.length;
    const ok = await confirmAction({
      title: hostId
        ? t("Deliver to this machine?")
        : t("Deliver to {n} machines?").replace("{n}", String(count)),
      description: t(
        "This changes the Claude Code and Codex files on the selected machines. Your own content in them stays. A copy of each file is kept the first time it is changed.",
      ),
      confirmLabel: t("Deliver"),
    });
    if (!ok) return;
    apply.mutate(hostId ? [hostId] : undefined, {
      onSuccess: (res) => {
        const failed = res.hosts.some((h) => h.items.some((it) => it.error));
        toast(failed ? t("Delivered, but some items failed") : t("Delivered"));
      },
      onError: showError,
    });
  };

  const hosts: AIConfigHostStatus[] | undefined = status.data?.hosts;
  const { drift, conflict } = problemCount(hosts);
  const subtitle =
    conflict > 0
      ? t("{n} machines have a conflict").replace("{n}", String(conflict))
      : drift > 0
        ? t("{n} machines are out of sync").replace("{n}", String(drift))
        : hosts && hosts.length > 0
          ? t("All machines are in sync")
          : undefined;

  return (
    <div className="xc-page aiconfig-page">
      <PageHeading
        title={t("Config delivery")}
        subtitle={subtitle}
        aside={
          <button
            className="xc-btn primary"
            disabled={!dirty || save.isPending}
            onClick={onSave}
          >
            <Save size={15} /> {t("Save")}
          </button>
        }
      />
      <p className="aiconfig-intro">
        {t(
          "Keep one Claude Code and Codex configuration here and write it to your machines. Only what you write here is managed. Your own settings on the machines are not read or changed.",
        )}
      </p>
      {dirty && (
        <p className="aiconfig-dirty" role="status">
          {t("You have unsaved changes")}
        </p>
      )}
      <HostsCard
        hosts={saved.hosts}
        selected={draft.hostIds}
        onToggle={toggleHost}
        statuses={hosts}
        checking={status.isFetching}
        canApply={!dirty}
        applying={apply.isPending}
        onCheck={() => void status.refetch()}
        onApply={(id) => void onApply(id)}
      />
      <ToolEditor
        tool="claude"
        value={draft.claude}
        onChange={(claude) => change({ ...draft, claude })}
      />
      <ToolEditor
        tool="codex"
        value={draft.codex}
        onChange={(codex) => change({ ...draft, codex })}
      />
    </div>
  );
}

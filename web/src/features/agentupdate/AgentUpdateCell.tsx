import { RefreshCw } from "lucide-react";
import { errorMessage } from "../../api/client";
import Switch from "../../components/ui/Switch";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useSetAutoUpdate, useUpdateAgentNow, type AgentUpdate } from "./api";
import "./agentupdate.css";
import "./i18n";

/** B233：设备页版本那一格。版本号后面标可更新、正在更新或失败原因。 */
export function AgentVersionCell({
  version,
  info,
  latest,
  online,
}: {
  version: string;
  info?: AgentUpdate;
  latest?: string;
  online: boolean;
}) {
  const t = useT();
  const update = useUpdateAgentNow();
  const outdated = !!latest && latest !== "dev" && version !== latest;
  const updating = info?.state === "updating" || update.isPending;
  return (
    <div className="agentupdate-cell">
      <span className="xc-mono">{version}</span>
      {updating ? (
        <span className="xc-badge accent">{t("Updating agent")}</span>
      ) : (
        outdated &&
        info && (
          <>
            {info.canUpdate ? (
              <>
                <span className="xc-badge warn">{t("Update available")}</span>
                <button
                  className="xc-btn small"
                  disabled={!online}
                  onClick={() =>
                    update.mutate(info.agentId, {
                      onSuccess: () => toast(t("Agent update started")),
                      onError: (e) =>
                        toast({ message: errorMessage(e), tone: "error" }),
                    })
                  }
                >
                  <RefreshCw size={13} /> {t("Update agent")}
                </button>
              </>
            ) : (
              <small className="xc-muted">
                {t(
                  "This agent cannot update itself. It is too old, or updates are off in its config. Update it by hand.",
                )}
              </small>
            )}
          </>
        )
      )}
      {!updating && info?.state === "failed" && info.error && (
        <small className="xc-error-text">
          {t("Agent update failed")}：{info.error}
        </small>
      )}
    </div>
  );
}

/** B233：设备页“自动更新”那一格。 */
export function AgentAutoUpdateCell({ info }: { info?: AgentUpdate }) {
  const t = useT();
  const set = useSetAutoUpdate();
  if (!info) return <span className="xc-muted">—</span>;
  return (
    <Switch
      checked={info.autoUpdate}
      disabled={set.isPending || !info.canUpdate}
      label={t("Auto update for this device")}
      onChange={(on) =>
        set.mutate(
          { agentId: info.agentId, on },
          {
            onError: (e) => toast({ message: errorMessage(e), tone: "error" }),
          },
        )
      }
    />
  );
}

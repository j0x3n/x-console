import { useEffect, useState } from "react";
import { BellRing } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { Loading, NotLive } from "../../components/ui/States";
import Switch from "../../components/ui/Switch";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useGitHubNotify, useSaveGitHubNotify, type RepoNotify } from "./api";
import { DEFAULT_NOTIFY, sameRepo, type RepoRow } from "./logic";
import NotifyFields from "./NotifyFields";

/** B71：一个仓库的通知。默认跟随设置里的默认值，关掉“跟随”后单独勾选。 */
export default function NotifyDialog({
  row,
  onClose,
}: {
  row: RepoRow;
  onClose: () => void;
}) {
  const t = useT();
  const settings = useGitHubNotify();
  const save = useSaveGitHubNotify();
  const sel = { repo: row.repo, connectionId: row.connectionId };
  const override = settings.data?.repos.find((r) => sameRepo(r, sel));
  const defaults = settings.data?.defaults ?? DEFAULT_NOTIFY;
  const [follow, setFollow] = useState(true);
  const [value, setValue] = useState<RepoNotify>(DEFAULT_NOTIFY);
  useEffect(() => {
    setFollow(!override);
    setValue(override?.notify ?? defaults);
  }, [settings.data]);

  const onSave = () => {
    if (!settings.data) return;
    const others = settings.data.repos.filter((r) => !sameRepo(r, sel));
    save.mutate(
      {
        defaults: settings.data.defaults,
        repos: follow
          ? others
          : [
              ...others,
              { connectionId: row.connectionId, repo: row.repo, notify: value },
            ],
      },
      {
        onSuccess: () => {
          toast(t("Saved"));
          onClose();
        },
        onError: (error) =>
          toast({ message: errorMessage(error), tone: "error" }),
      },
    );
  };

  return (
    <Dialog open onClose={onClose} title={`${t("Notifications")}：${row.repo}`}>
      {settings.isPending ? (
        <Loading />
      ) : settings.isError ? (
        isNotLive(settings.error) ? (
          <NotLive
            name={t("Repository notifications")}
            icon={<BellRing size={28} />}
          />
        ) : (
          <p className="xc-error-text">{errorMessage(settings.error)}</p>
        )
      ) : (
        <div className="xc-stack">
          <div className="repos-notify-follow">
            <Switch
              checked={follow}
              onChange={setFollow}
              label={t("Use the default settings")}
            />
            <span>{t("Use the default settings")}</span>
          </div>
          <NotifyFields
            value={follow ? defaults : value}
            onChange={setValue}
            disabled={follow}
          />
          <small className="xc-muted">
            {t(
              "Where notifications go is set in Settings → Notifications. Their kinds start with github.",
            )}
          </small>
        </div>
      )}
      <div className="xc-dialog-actions">
        <button type="button" className="xc-btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button
          type="button"
          className="xc-btn primary"
          disabled={!settings.data || save.isPending}
          onClick={onSave}
        >
          {t("Save")}
        </button>
      </div>
    </Dialog>
  );
}

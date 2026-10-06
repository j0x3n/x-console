import { useEffect, useState } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateQuotaAccount,
  useQuotaHosts,
  useUpdateQuotaAccount,
  type QuotaAccount,
  type QuotaKind,
} from "./api";
import { KIND_NAMES, KIND_ORDER } from "./format";

/** 每种服务在这台机器上用单独的目录登录的方法。命令照原样显示，不翻译。 */
const LOGIN_HINTS: Record<
  Exclude<QuotaKind, "deepseek">,
  { dir: string; command: string }
> = {
  claude: {
    dir: "~/.claude",
    command: "CLAUDE_CONFIG_DIR=~/.claude-work claude auth login",
  },
  codex: { dir: "~/.codex", command: "CODEX_HOME=~/.codex-work codex login" },
  grok: { dir: "~/.grok", command: "GROK_HOME=~/.grok-work grok login" },
};

/** 添加或修改一个账号。传了 account 就是修改。 */
export default function AccountDialog({
  open,
  account,
  onClose,
}: {
  open: boolean;
  account?: QuotaAccount;
  onClose: () => void;
}) {
  const t = useT();
  const editing = !!account;
  const [kind, setKind] = useState<QuotaKind>("claude");
  const [name, setName] = useState("");
  const [hostId, setHostId] = useState("");
  const [home, setHome] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [balanceLow, setBalanceLow] = useState("");
  const hosts = useQuotaHosts(open && kind !== "deepseek");
  const create = useCreateQuotaAccount();
  const update = useUpdateQuotaAccount();
  const pending = create.isPending || update.isPending;

  useEffect(() => {
    if (!open) return;
    setKind(account?.kind ?? "claude");
    setName(account?.name ?? "");
    setHostId(account?.hostId ?? "");
    setHome(account?.home ?? "");
    setApiKey("");
    setBalanceLow(account?.balanceLow ?? "");
  }, [open, account]);

  // 只有一台机器时直接选上
  useEffect(() => {
    if (open && !editing && !hostId && hosts.data?.length === 1)
      setHostId(hosts.data[0]!.id);
  }, [open, editing, hostId, hosts.data]);

  const isKey = kind === "deepseek";
  const hint = isKey ? null : LOGIN_HINTS[kind];
  const valid =
    name.trim() !== "" && (isKey ? editing || apiKey.trim() !== "" : !!hostId);

  const submit = () => {
    const fail = (e: unknown) =>
      toast({ message: errorMessage(e), tone: "error" });
    const done = () => {
      toast(t("Saved"));
      onClose();
    };
    if (account) {
      update.mutate(
        {
          id: account.id,
          body: {
            name: name.trim(),
            ...(isKey
              ? {
                  ...(apiKey.trim() ? { apiKey: apiKey.trim() } : {}),
                  balanceLow: balanceLow.trim(),
                }
              : { hostId, home: home.trim() }),
          },
        },
        { onSuccess: done, onError: fail },
      );
      return;
    }
    create.mutate(
      {
        kind,
        name: name.trim(),
        ...(isKey
          ? {
              apiKey: apiKey.trim(),
              ...(balanceLow.trim() ? { balanceLow: balanceLow.trim() } : {}),
            }
          : { hostId, ...(home.trim() ? { home: home.trim() } : {}) }),
      },
      { onSuccess: done, onError: fail },
    );
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={editing ? t("Edit quota account") : t("Add quota account")}
      footer={
        <>
          <button className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={!valid || pending}
            onClick={submit}
          >
            {t("Save")}
          </button>
        </>
      }
    >
      <form
        className="quota-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (valid && !pending) submit();
        }}
      >
        <div className="xc-field">
          <label htmlFor="quota-kind">{t("Service")}</label>
          <select
            id="quota-kind"
            className="xc-select"
            value={kind}
            disabled={editing}
            onChange={(e) => {
              setKind(e.target.value as QuotaKind);
              setHostId("");
              setHome("");
            }}
          >
            {KIND_ORDER.map((k) => (
              <option key={k} value={k}>
                {KIND_NAMES[k]}
              </option>
            ))}
          </select>
        </div>
        <div className="xc-field">
          <label htmlFor="quota-name">{t("Account name")}</label>
          <input
            id="quota-name"
            className="xc-input"
            value={name}
            maxLength={60}
            placeholder={t("For example: personal, work")}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        {isKey ? (
          <>
            <div className="xc-field">
              <label htmlFor="quota-key">{t("DeepSeek API key")}</label>
              <input
                id="quota-key"
                className="xc-input"
                type="password"
                autoComplete="off"
                value={apiKey}
                placeholder={
                  editing ? t("Leave empty to keep the current key") : "sk-…"
                }
                onChange={(e) => setApiKey(e.target.value)}
              />
              <small>
                {t("The key is stored encrypted and never shown again.")}
              </small>
            </div>
            <div className="xc-field">
              <label htmlFor="quota-balance-low">
                {t("Notify when below")}
              </label>
              <input
                id="quota-balance-low"
                className="xc-input"
                inputMode="decimal"
                value={balanceLow}
                placeholder={t("Empty means no notification")}
                onChange={(e) => setBalanceLow(e.target.value)}
              />
              <small>
                {t("Compared with the first currency of the balance.")}
              </small>
            </div>
          </>
        ) : (
          <>
            <div className="xc-field">
              <label htmlFor="quota-host">{t("Machine")}</label>
              <select
                id="quota-host"
                className="xc-select"
                value={hostId}
                onChange={(e) => setHostId(e.target.value)}
              >
                <option value="">{t("Choose a machine")}</option>
                {(hosts.data ?? []).map((h) => (
                  <option key={h.id} value={h.id}>
                    {h.name}
                    {h.online ? "" : ` (${t("Offline")})`}
                  </option>
                ))}
              </select>
              {hosts.data?.length === 0 && (
                <small>
                  {t(
                    "No machine can read quotas. Install Claude Code, Codex or Grok on a machine with an agent, and update the agent.",
                  )}
                </small>
              )}
            </div>
            <div className="xc-field">
              <label htmlFor="quota-home">{t("Sign-in directory")}</label>
              <input
                id="quota-home"
                className="xc-input xc-mono"
                value={home}
                placeholder={hint?.dir}
                onChange={(e) => setHome(e.target.value)}
              />
              <small>
                {t(
                  "Leave empty for the default directory. Use an absolute path, or one that starts with ~/.",
                )}
              </small>
              {hint && (
                <small>
                  {t("To sign in a second account in its own directory, run:")}{" "}
                  <code>{hint.command}</code>
                </small>
              )}
            </div>
          </>
        )}
      </form>
    </Dialog>
  );
}

import { useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { relativeTime } from "../../lib/time";
import {
  useClearXAuth,
  useSaveXAuth,
  useTestXAuth,
  useXAuth,
  type XAuth,
} from "./api";
import "./i18n";

const STATUS: Record<
  XAuth["status"],
  { label: string; tone: "ok" | "warn" | "danger" | "info" | "" }
> = {
  none: { label: "No cookie yet", tone: "" },
  unverified: { label: "Not checked yet", tone: "info" },
  ok: { label: "Works", tone: "ok" },
  expired: { label: "May have expired", tone: "warn" },
  error: { label: "Last test failed", tone: "danger" },
};

function showError(error: unknown) {
  toast({ message: errorMessage(error), tone: "error" });
}

/** 设置 → 稍后阅读（B143）：读 X 推文用的 Cookie 和备用方案。 */
export default function XSettingsTab() {
  const query = useXAuth();
  if (query.isPending) return <Loading />;
  if (query.isError)
    return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  return (
    <div className="settings-grid">
      <XCard state={query.data} />
    </div>
  );
}

function XCard({ state }: { state: XAuth }) {
  const t = useT();
  const language = useLanguage();
  const save = useSaveXAuth();
  const clear = useClearXAuth();
  const test = useTestXAuth();
  const [authToken, setAuthToken] = useState("");
  const [ct0, setCt0] = useState("");
  const [fx, setFx] = useState(state.fxtwitter);
  const [queryId, setQueryId] = useState(state.queryId);
  const status = STATUS[state.status];

  const onSave = (e: FormEvent) => {
    e.preventDefault();
    const a = authToken.trim();
    const c = ct0.trim();
    if (Boolean(a) !== Boolean(c)) {
      toast({ message: t("Enter both auth_token and ct0"), tone: "error" });
      return;
    }
    withElevation(() =>
      save.mutateAsync({
        fxtwitter: fx,
        queryId: queryId.trim(),
        ...(a ? { authToken: a, ct0: c } : {}),
      }),
    ).then(() => {
      setAuthToken("");
      setCt0("");
      toast(t("X settings saved"));
    }, showError);
  };

  const onClear = async () => {
    const ok = await confirmAction({
      title: t("Remove the X cookie?"),
      description: t(
        "Long posts and posts that need a login will not be read until you add a cookie again.",
      ),
      confirmLabel: t("Remove cookie"),
    });
    if (!ok) return;
    withElevation(() => clear.mutateAsync()).then(
      () => toast(t("Cookie removed")),
      showError,
    );
  };

  const onTest = () =>
    test.mutate(undefined, {
      onSuccess: (data) =>
        data.status === "ok"
          ? toast(t("Cookie works"))
          : toast({
              message: data.message || t(STATUS[data.status].label),
              tone: "error",
            }),
      onError: showError,
    });

  return (
    <form className="xc-card" onSubmit={onSave}>
      <div className="xc-card-head">
        <h2>{t("Reading X posts")}</h2>
        <span className={`xc-badge ${status.tone}`}>{t(status.label)}</span>
      </div>
      <p className="xc-muted">
        {t(
          "X posts need more than the page itself. The server tries the public interface first. If a post is cut short or needs a login, it uses your cookie.",
        )}
      </p>
      {state.configured && state.verifiedAt && (
        <p className="xc-muted">
          {t("Last checked")}：{relativeTime(state.verifiedAt, language)}
        </p>
      )}
      {state.configured && state.status !== "ok" && state.message && (
        <p className="xc-error-text">{state.message}</p>
      )}
      <label className="xc-field">
        <span>{t("auth_token value")}</span>
        <input
          className="xc-input"
          type="password"
          value={authToken}
          onChange={(e) => setAuthToken(e.target.value)}
          placeholder={
            state.configured ? t("Leave empty to keep the saved cookie") : ""
          }
          autoComplete="off"
        />
      </label>
      <label className="xc-field">
        <span>{t("ct0 value")}</span>
        <input
          className="xc-input"
          type="password"
          value={ct0}
          onChange={(e) => setCt0(e.target.value)}
          placeholder={
            state.configured ? t("Leave empty to keep the saved cookie") : ""
          }
          autoComplete="off"
        />
        <small>
          {t(
            "How to find them: sign in to x.com in your browser, open the developer tools, then Application → Cookies → https://x.com. Copy the values of auth_token and ct0. Use a spare account. This goes against X's terms and the account may be limited.",
          )}
        </small>
      </label>
      <label className="xc-check">
        <input
          type="checkbox"
          checked={fx}
          onChange={(e) => setFx(e.target.checked)}
        />
        <span>{t("Use a third-party converter as the last step")}</span>
      </label>
      <small className="xc-check-hint">
        {t(
          "When the two steps above fail, the post link is sent to api.fxtwitter.com. Off by default.",
        )}
      </small>
      <label className="xc-field">
        <span>{t("GraphQL queryId")}</span>
        <input
          className="xc-input"
          value={queryId}
          onChange={(e) => setQueryId(e.target.value)}
          autoComplete="off"
          spellCheck={false}
        />
        <small>
          {t(
            "Leave empty to use the built-in one. Change it only when the test says the interface changed.",
          )}
        </small>
      </label>
      <div className="xc-dialog-actions">
        {state.configured && (
          <button
            type="button"
            className="xc-btn ghost danger"
            disabled={clear.isPending}
            onClick={() => void onClear()}
          >
            {t("Remove cookie")}
          </button>
        )}
        <span className="xc-spacer" />
        {state.configured && (
          <button
            type="button"
            className="xc-btn"
            disabled={test.isPending}
            onClick={onTest}
          >
            {t("Test cookie")}
          </button>
        )}
        <button
          type="submit"
          className="xc-btn primary"
          disabled={save.isPending}
        >
          {t("Save X settings")}
        </button>
      </div>
    </form>
  );
}

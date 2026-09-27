import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { coreApi, coreKeys, useAuthStatus } from "../api/core";
import { errorMessage, unwrap } from "../api/client";
import Dialog from "../components/ui/Dialog";
import { useT } from "../contexts/LanguageContext";
import { useElevationStore } from "./elevation";

/*
 * 高危操作前再验证一次。开了两步验证输入验证码，没开输入登录密码。
 * 服务端还没返回 totpEnabled 时（老版本），按开了处理。
 */
export default function ElevationDialog() {
  const t = useT();
  const qc = useQueryClient();
  const status = useAuthStatus();
  const useCode = status.data?.totpEnabled ?? true;
  const open = useElevationStore((s) => s.open);
  const finish = useElevationStore((s) => s.finish);
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const close = (ok: boolean) => {
    setValue("");
    setError("");
    finish(ok);
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    try {
      await unwrap(
        coreApi.POST("/auth/elevate", {
          body: useCode ? { code: value } : { password: value },
        }),
      );
      qc.invalidateQueries({ queryKey: coreKeys.auth });
      close(true);
    } catch (err) {
      setError(errorMessage(err));
      setValue("");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      open={open}
      onClose={() => close(false)}
      title={t("Verify it's you")}
      description={
        useCode
          ? t("Enter the 6-digit code from your authenticator app.")
          : t("Enter your login password.")
      }
    >
      <form onSubmit={submit}>
        {useCode ? (
          <label className="xc-field">
            <span>{t("Verification code")}</span>
            <input
              className="xc-input xc-mono"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              maxLength={6}
              autoFocus
              value={value}
              onChange={(e) => setValue(e.target.value.trim())}
              required
            />
          </label>
        ) : (
          <label className="xc-field">
            <span>{t("Password")}</span>
            <input
              className="xc-input"
              type="password"
              autoComplete="current-password"
              autoFocus
              value={value}
              onChange={(e) => setValue(e.target.value)}
              required
            />
          </label>
        )}
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button
            type="button"
            className="xc-btn ghost"
            onClick={() => close(false)}
          >
            {t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={busy}>
            {t("Verify")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { coreApi, coreKeys } from "../api/core";
import { errorMessage, unwrap } from "../api/client";
import Dialog from "../components/ui/Dialog";
import { useT } from "../contexts/LanguageContext";
import { useElevationStore } from "./elevation";

export default function ElevationDialog() {
  const t = useT();
  const qc = useQueryClient();
  const open = useElevationStore((s) => s.open);
  const finish = useElevationStore((s) => s.finish);
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const close = (ok: boolean) => {
    setCode("");
    setError("");
    finish(ok);
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    try {
      await unwrap(coreApi.POST("/auth/elevate", { body: { code } }));
      qc.invalidateQueries({ queryKey: coreKeys.auth });
      close(true);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      open={open}
      onClose={() => close(false)}
      title={t("Verify it's you")}
      description={t("Enter the 6-digit code from your authenticator app.")}
    >
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Verification code")}</span>
          <input
            className="xc-input xc-mono"
            inputMode="numeric"
            pattern="[0-9]{6}"
            maxLength={6}
            autoFocus
            value={code}
            onChange={(e) => setCode(e.target.value.trim())}
            required
          />
        </label>
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

import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ShieldCheck, ShieldOff } from "lucide-react";
import { coreApi, coreKeys, useAuthStatus } from "../../api/core";
import { errorMessage, unwrap } from "../../api/client";
import { withElevation } from "../../auth/elevation";
import TotpQr from "../../auth/TotpQr";
import Dialog from "../../components/ui/Dialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import VaultPasswordCard from "../vault/VaultPasswordCard";
import ElevationModeCard from "./ElevationModeCard";
import { MIN_PASSWORD, passwordFormError } from "./security";

/** 设置里的“安全”标签：两步验证开关、修改登录密码。 */
export default function SecurityTab() {
  const status = useAuthStatus();
  if (status.isPending) return <Loading />;
  if (status.isError)
    return <ErrorState error={status.error} onRetry={() => status.refetch()} />;
  // 老版本服务端不返回 totpEnabled，那时两步验证是必开的。
  const enabled = status.data.totpEnabled ?? true;
  return (
    <div className="settings-grid">
      <TotpCard enabled={enabled} />
      <PasswordCard />
      <ElevationModeCard />
      <VaultPasswordCard />
    </div>
  );
}

function TotpCard({ enabled }: { enabled: boolean }) {
  const t = useT();
  const qc = useQueryClient();
  const [enrollment, setEnrollment] = useState<{
    secret: string;
    otpauthUrl: string;
  } | null>(null);
  const [code, setCode] = useState("");
  const [disabling, setDisabling] = useState(false);

  const enroll = useMutation({
    mutationFn: () =>
      withElevation(() => unwrap(coreApi.POST("/auth/totp/enroll"))),
    onSuccess: (result) => {
      setCode("");
      setEnrollment(result);
    },
    onError: (error) => toast({ message: errorMessage(error), tone: "error" }),
  });
  const confirm = useMutation({
    mutationFn: (value: string) =>
      unwrap(coreApi.POST("/auth/totp/confirm", { body: { code: value } })),
    onSuccess: () => {
      setEnrollment(null);
      setCode("");
      qc.invalidateQueries({ queryKey: coreKeys.auth });
      toast("两步验证已开启");
    },
    onError: () => setCode(""),
  });

  const onConfirm = (event: FormEvent) => {
    event.preventDefault();
    confirm.mutate(code);
  };

  return (
    <div className="xc-card">
      <div className="xc-card-head">
        <h2>{t("Two-step verification")}</h2>
        <span className={`xc-badge ${enabled ? "ok" : ""}`}>
          {enabled ? t("2FA on") : t("2FA off")}
        </span>
      </div>
      <p className="settings-note">
        {enabled
          ? "登录和高危操作都要输入验证器 App 里的 6 位验证码。"
          : "现在只用密码登录，高危操作要再输一次密码。开启后，这两处都改成输入验证码。"}
      </p>

      {enabled ? (
        <button
          className="xc-btn danger"
          type="button"
          onClick={() => setDisabling(true)}
        >
          <ShieldOff size={15} />
          {t("Turn off two-step verification")}
        </button>
      ) : enrollment ? (
        <form onSubmit={onConfirm}>
          <p className="settings-note">
            用验证器 App（比如 Google Authenticator 或
            1Password）扫描二维码，再输入它显示的 6 位数字。确认之后才会生效。
          </p>
          <TotpQr
            otpauthUrl={enrollment.otpauthUrl}
            secret={enrollment.secret}
          />
          <label className="xc-field">
            <span>{t("Verification code")}</span>
            <input
              className="xc-input xc-mono"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              maxLength={6}
              autoFocus
              value={code}
              onChange={(e) => setCode(e.target.value.trim())}
              required
            />
          </label>
          {confirm.isError && (
            <p className="xc-error-text">{errorMessage(confirm.error)}</p>
          )}
          <div className="settings-actions">
            <button
              type="button"
              className="xc-btn ghost"
              onClick={() => {
                setEnrollment(null);
                confirm.reset();
              }}
            >
              {t("Cancel")}
            </button>
            <button className="xc-btn primary" disabled={confirm.isPending}>
              {t("Confirm")}
            </button>
          </div>
        </form>
      ) : (
        <button
          className="xc-btn primary"
          type="button"
          disabled={enroll.isPending}
          onClick={() => enroll.mutate()}
        >
          <ShieldCheck size={15} />
          {t("Turn on two-step verification")}
        </button>
      )}

      <DisableTotpDialog open={disabling} onClose={() => setDisabling(false)} />
    </div>
  );
}

function DisableTotpDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const qc = useQueryClient();
  const [form, setForm] = useState({ password: "", code: "" });
  const disable = useMutation({
    mutationFn: (body: { password: string; code: string }) =>
      unwrap(coreApi.POST("/auth/totp/disable", { body })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: coreKeys.auth });
      toast("两步验证已关闭");
      close();
    },
    onError: () => setForm((f) => ({ ...f, code: "" })),
  });
  const close = () => {
    setForm({ password: "", code: "" });
    disable.reset();
    onClose();
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    disable.mutate(form);
  };
  return (
    <Dialog
      open={open}
      onClose={close}
      title={t("Turn off two-step verification")}
      description="关闭后只用密码登录。要输入密码和当前的验证码。"
    >
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Password")}</span>
          <input
            className="xc-input"
            type="password"
            autoComplete="current-password"
            autoFocus
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required
          />
        </label>
        <label className="xc-field">
          <span>{t("Verification code")}</span>
          <input
            className="xc-input xc-mono"
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]{6}"
            maxLength={6}
            value={form.code}
            onChange={(e) => setForm({ ...form, code: e.target.value.trim() })}
            required
          />
        </label>
        {disable.isError && (
          <p className="xc-error-text">{errorMessage(disable.error)}</p>
        )}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={close}>
            {t("Cancel")}
          </button>
          <button className="xc-btn danger" disabled={disable.isPending}>
            {t("Turn off two-step verification")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function PasswordCard() {
  const t = useT();
  const empty = { oldPassword: "", newPassword: "", confirm: "" };
  const [form, setForm] = useState(empty);
  const [error, setError] = useState("");
  const change = useMutation({
    mutationFn: (body: { oldPassword: string; newPassword: string }) =>
      unwrap(coreApi.POST("/auth/password", { body })),
    onSuccess: () => {
      setForm(empty);
      toast("密码已修改，其他设备要重新登录");
    },
    onError: (err) => setError(errorMessage(err)),
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    const problem = passwordFormError(form);
    setError(problem);
    if (problem) return;
    change.mutate({
      oldPassword: form.oldPassword,
      newPassword: form.newPassword,
    });
  };
  return (
    <form className="xc-card" onSubmit={submit}>
      <div className="xc-card-head">
        <h2>{t("Login password")}</h2>
      </div>
      <p className="settings-note">改完之后，其他设备上的登录会全部退出。</p>
      <label className="xc-field">
        <span>{t("Current password")}</span>
        <input
          className="xc-input"
          type="password"
          autoComplete="current-password"
          value={form.oldPassword}
          onChange={(e) => setForm({ ...form, oldPassword: e.target.value })}
          required
        />
      </label>
      <label className="xc-field">
        <span>{t("New password")}</span>
        <input
          className="xc-input"
          type="password"
          autoComplete="new-password"
          minLength={MIN_PASSWORD}
          value={form.newPassword}
          onChange={(e) => setForm({ ...form, newPassword: e.target.value })}
          required
        />
        <small>至少 {MIN_PASSWORD} 位</small>
      </label>
      <label className="xc-field">
        <span>{t("Repeat new password")}</span>
        <input
          className="xc-input"
          type="password"
          autoComplete="new-password"
          value={form.confirm}
          onChange={(e) => setForm({ ...form, confirm: e.target.value })}
          required
        />
      </label>
      {error && <p className="xc-error-text">{error}</p>}
      <div className="settings-actions">
        <button className="xc-btn primary" disabled={change.isPending}>
          {t("Change password")}
        </button>
      </div>
    </form>
  );
}

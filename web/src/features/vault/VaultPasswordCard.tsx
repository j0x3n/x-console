import { useState, type FormEvent } from "react";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useChangeVaultPassword, useVaultStatus } from "./api";
import { MIN_VAULT_PASSWORD, vaultPasswordError } from "./logic";
import "./i18n";

/*
 * 设置 → 安全里的“隐藏密码”卡片。只在设过隐藏密码后出现，
 * 没设过时不显示，免得暴露入口。
 */
export default function VaultPasswordCard() {
  const t = useT();
  const status = useVaultStatus();
  const change = useChangeVaultPassword();
  const empty = { oldPassword: "", newPassword: "", confirm: "" };
  const [form, setForm] = useState(empty);
  const [error, setError] = useState("");
  if (!status.data?.available || !status.data.configured) return null;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const problem = vaultPasswordError(form.newPassword, form.confirm);
    setError(problem);
    if (problem) return;
    change.mutate(
      { oldPassword: form.oldPassword, newPassword: form.newPassword },
      {
        onSuccess: () => {
          setForm(empty);
          toast("隐藏密码已修改");
        },
        onError: (err) => setError(errorMessage(err)),
      },
    );
  };

  return (
    <form className="xc-card" onSubmit={submit}>
      <div className="xc-card-head">
        <h2>{t("Vault password")}</h2>
      </div>
      <p className="vault-note">
        忘了只能在服务器上运行 x-console-server reset-vault-password 重置。
      </p>
      <label className="xc-field">
        <span>{t("Current password")}</span>
        <input
          className="xc-input"
          type="password"
          autoComplete="off"
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
          minLength={MIN_VAULT_PASSWORD}
          value={form.newPassword}
          onChange={(e) => setForm({ ...form, newPassword: e.target.value })}
          required
        />
        <small>至少 {MIN_VAULT_PASSWORD} 位</small>
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
      <div className="vault-actions">
        <button className="xc-btn primary" disabled={change.isPending}>
          {t("Change vault password")}
        </button>
      </div>
    </form>
  );
}

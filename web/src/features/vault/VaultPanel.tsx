import { useEffect, useRef, useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
import { LockOpen } from "lucide-react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import {
  useLockVault,
  useSetupVault,
  useUnlockVault,
  useVaultStatus,
  type VaultState,
} from "./api";
import {
  BRAND_TAP_EVENT,
  IDLE_LOCK_MS,
  MIN_VAULT_PASSWORD,
  hasVaultSession,
  registerTap,
  vaultPasswordError,
} from "./logic";
import "./i18n";
import "./vault.css";

/*
 * 隐藏内容的全局部分（B13）：
 * - 连续点 Logo 3 次弹出密码框，界面上没有别的入口提示。
 * - 解锁后只在侧边栏“X Console”后面显示一个小锁图标，点一下锁定，图标消失。
 * - 15 分钟没有操作、关掉页面后重新打开，都会自动锁定。
 */
export default function VaultPanel() {
  const status = useVaultStatus();
  const lock = useLockVault();
  const [open, setOpen] = useState(false);
  const taps = useRef<number[]>([]);
  const unlocked = status.data?.unlocked ?? false;

  useEffect(() => {
    const onTap = () => {
      const next = registerTap(taps.current, Date.now());
      taps.current = next.taps;
      if (next.fire) setOpen(true);
    };
    window.addEventListener(BRAND_TAP_EVENT, onTap);
    return () => window.removeEventListener(BRAND_TAP_EVENT, onTap);
  }, []);

  // 重新打开页面时服务端还是解锁状态，锁上。
  const checked = useRef(false);
  useEffect(() => {
    if (checked.current || !status.data) return;
    checked.current = true;
    if (status.data.unlocked && !hasVaultSession()) lock.mutate();
  }, [status.data]);

  useIdleLock(unlocked, () => lock.mutate());

  return (
    <>
      {unlocked && (
        <VaultBar onLock={() => lock.mutate()} busy={lock.isPending} />
      )}
      {open && status.data && (
        <VaultDialog state={status.data} onClose={() => setOpen(false)} />
      )}
    </>
  );
}

/** 解锁状态下，一段时间没有点击、按键、滚动就锁定。 */
function useIdleLock(active: boolean, onIdle: () => void) {
  const last = useRef(Date.now());
  const idle = useRef(onIdle);
  idle.current = onIdle;
  useEffect(() => {
    if (!active) return;
    last.current = Date.now();
    const touch = () => {
      last.current = Date.now();
    };
    const events = ["pointerdown", "keydown", "wheel", "touchstart"];
    events.forEach((e) =>
      window.addEventListener(e, touch, { passive: true, capture: true }),
    );
    // 用轮询比较时间，页面在后台被节流后回来也能马上判断。
    const check = () => {
      if (Date.now() - last.current >= IDLE_LOCK_MS) idle.current();
    };
    const timer = window.setInterval(check, 15_000);
    document.addEventListener("visibilitychange", check);
    return () => {
      events.forEach((e) => window.removeEventListener(e, touch, true));
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", check);
    };
  }, [active]);
}

function VaultBar({ onLock, busy }: { onLock: () => void; busy: boolean }) {
  const t = useT();
  const host = document.getElementById("brand-slot");
  const button = (
    <button
      type="button"
      className="vault-lock"
      onClick={onLock}
      disabled={busy}
      title={t("Lock vault")}
      aria-label={t("Lock vault")}
    >
      <LockOpen size={13} />
    </button>
  );
  return host ? createPortal(button, host) : null;
}

function VaultDialog({
  state,
  onClose,
}: {
  state: VaultState;
  onClose: () => void;
}) {
  const t = useT();
  const setup = useSetupVault();
  const unlock = useUnlockVault();
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");

  if (!state.available)
    return (
      <Dialog open onClose={onClose} title={t("Hidden items")}>
        <p className="vault-note">这个功能还没上线。</p>
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Close")}
          </button>
        </div>
      </Dialog>
    );

  if (state.unlocked)
    return (
      <Dialog open onClose={onClose} title={t("Hidden items")}>
        <p className="vault-note">隐藏内容已经显示了。</p>
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Close")}
          </button>
        </div>
      </Dialog>
    );

  const creating = !state.configured;
  const busy = setup.isPending || unlock.isPending;
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (creating) {
      const problem = vaultPasswordError(password, confirm);
      setError(problem);
      if (problem) return;
    }
    setError("");
    const mutation = creating ? setup : unlock;
    mutation.mutate(password, {
      onSuccess: onClose,
      onError: (err) => {
        setError(errorMessage(err));
        setPassword("");
        setConfirm("");
      },
    });
  };

  return (
    <Dialog
      open
      onClose={onClose}
      title={creating ? t("Set a vault password") : t("Hidden items")}
      description={
        creating
          ? `以后输入这个密码才能看到隐藏的笔记和文件。至少 ${MIN_VAULT_PASSWORD} 位。`
          : "输入隐藏密码。"
      }
    >
      <form onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Vault password")}</span>
          <input
            className="xc-input"
            type="password"
            autoComplete={creating ? "new-password" : "off"}
            autoFocus
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </label>
        {creating && (
          <label className="xc-field">
            <span>{t("Repeat the password")}</span>
            <input
              className="xc-input"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              required
            />
          </label>
        )}
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button className="xc-btn primary" disabled={busy}>
            {creating ? t("Save") : t("Unlock vault")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

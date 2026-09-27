import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import { coreApi, coreKeys, useAuthStatus } from "../api/core";
import { ApiError, errorMessage, onUnauthorized, unwrap } from "../api/client";
import { Loading, ErrorState } from "../components/ui/States";
import TotpQr from "./TotpQr";

/** 未初始化显示初始化页，未登录显示登录页，已登录渲染 children。 */
export default function AuthGate({ children }: { children: ReactNode }) {
  const qc = useQueryClient();
  const status = useAuthStatus();
  useEffect(
    () =>
      onUnauthorized(() => qc.invalidateQueries({ queryKey: coreKeys.auth })),
    [qc],
  );
  // 断网时不要一直转圈，也不要显示看不懂的错误。连上网后会自动重试。
  const offline =
    status.fetchStatus === "paused" ||
    (typeof navigator !== "undefined" && navigator.onLine === false);
  if ((status.isPending || status.isError) && offline)
    return (
      <div className="xc-auth">
        <div className="xc-auth-card">
          <Brand />
          <p>网络断开了。连上网后会自动继续。</p>
        </div>
      </div>
    );
  if (status.isPending) return <Loading />;
  if (status.isError)
    return <ErrorState error={status.error} onRetry={() => status.refetch()} />;
  if (status.data.setupRequired) return <SetupPage />;
  if (!status.data.authenticated) return <LoginPage />;
  return <>{children}</>;
}

function Brand() {
  return (
    <h1>
      <span className="brand-mark">X</span> X Console
    </h1>
  );
}

/*
 * 登录分两步：先交用户名和密码。开了两步验证的账号，服务端回 totp_required，
 * 这时才显示验证码输入框，再交一次。
 */
function LoginPage() {
  const qc = useQueryClient();
  const [form, setForm] = useState({ username: "", password: "", code: "" });
  const [needCode, setNeedCode] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const codeRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (needCode) codeRef.current?.focus();
  }, [needCode]);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await unwrap(
        coreApi.POST("/auth/login", {
          body: {
            username: form.username,
            password: form.password,
            code: needCode ? form.code : undefined,
          },
        }),
      );
      await qc.invalidateQueries({ queryKey: coreKeys.auth });
    } catch (err) {
      if (err instanceof ApiError && err.code === "totp_required") {
        setNeedCode(true);
      } else {
        setError(errorMessage(err));
      }
      setForm((f) => ({ ...f, code: "" }));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="xc-auth">
      <form className="xc-auth-card" onSubmit={submit}>
        <Brand />
        <p>登录你的个人控制台。</p>
        <label className="xc-field">
          <span>用户名</span>
          <input
            className="xc-input"
            autoComplete="username"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
            required
            autoFocus
          />
        </label>
        <label className="xc-field">
          <span>密码</span>
          <input
            className="xc-input"
            type="password"
            autoComplete="current-password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required
          />
        </label>
        {needCode && (
          <label className="xc-field">
            <span>两步验证码</span>
            <input
              ref={codeRef}
              className="xc-input xc-mono"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              maxLength={6}
              value={form.code}
              onChange={(e) =>
                setForm({ ...form, code: e.target.value.trim() })
              }
              required
            />
            <small>这个账号开了两步验证，输入验证器 App 里的 6 位数字。</small>
          </label>
        )}
        {error && <p className="xc-error-text">{error}</p>}
        <button className="xc-btn primary" disabled={busy}>
          登录
        </button>
      </form>
    </div>
  );
}

function SetupPage() {
  const qc = useQueryClient();
  const [step, setStep] = useState<"account" | "totp">("account");
  const [form, setForm] = useState({ username: "", password: "", confirm: "" });
  const [enrollment, setEnrollment] = useState<{
    secret: string;
    otpauthUrl: string;
  } | null>(null);
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const createAccount = async (event: FormEvent) => {
    event.preventDefault();
    if (form.password !== form.confirm) {
      setError("两次输入的密码不一致");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await unwrap(
        coreApi.POST("/auth/setup", {
          body: { username: form.username, password: form.password },
        }),
      );
      setEnrollment(result);
      setStep("totp");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const confirm = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await unwrap(coreApi.POST("/auth/setup/confirm", { body: { code } }));
      await qc.invalidateQueries({ queryKey: coreKeys.auth });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  // 跳过两步验证，直接登录。以后可以在设置的“安全”标签里开。
  const skip = async () => {
    setBusy(true);
    setError("");
    try {
      await unwrap(coreApi.POST("/auth/setup/skip-totp"));
      await qc.invalidateQueries({ queryKey: coreKeys.auth });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  if (step === "totp" && enrollment)
    return (
      <div className="xc-auth">
        <form className="xc-auth-card" onSubmit={confirm}>
          <Brand />
          <p>
            开启两步验证：用验证器 App（比如 Google Authenticator 或
            1Password）扫描二维码，再输入它显示的 6
            位数字。不想开可以跳过，以后在设置里也能开。
          </p>
          <TotpQr
            otpauthUrl={enrollment.otpauthUrl}
            secret={enrollment.secret}
          />
          <label className="xc-field">
            <span>验证码</span>
            <input
              className="xc-input xc-mono"
              inputMode="numeric"
              pattern="[0-9]{6}"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.trim())}
              required
              autoFocus
            />
          </label>
          {error && <p className="xc-error-text">{error}</p>}
          <div className="xc-auth-actions">
            <button className="xc-btn primary" disabled={busy}>
              开启并进入
            </button>
            <button
              type="button"
              className="xc-btn ghost"
              disabled={busy}
              onClick={skip}
            >
              跳过，以后再开
            </button>
          </div>
        </form>
      </div>
    );

  return (
    <div className="xc-auth">
      <form className="xc-auth-card" onSubmit={createAccount}>
        <Brand />
        <p>第一次使用，先创建你的账号。这个面板只有你一个用户。</p>
        <label className="xc-field">
          <span>用户名</span>
          <input
            className="xc-input"
            autoComplete="username"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
            required
            autoFocus
          />
        </label>
        <label className="xc-field">
          <span>密码</span>
          <input
            className="xc-input"
            type="password"
            autoComplete="new-password"
            minLength={10}
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            required
          />
          <small>至少 10 位</small>
        </label>
        <label className="xc-field">
          <span>再输一次密码</span>
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
        <button className="xc-btn primary" disabled={busy}>
          下一步
        </button>
      </form>
    </div>
  );
}

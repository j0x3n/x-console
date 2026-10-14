import { useEffect, useState } from "react";
import { errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useCreateWatcher,
  useUpdateWatcher,
  type Watcher,
  type WatcherInput,
  type WatcherPatch,
} from "./api";
import { INTERVALS, KEEPALIVES, minutesText, parseHosts } from "./lib";

/** 新建或修改一个盯梢任务。传了 watcher 就是修改。 */
export default function WatcherDialog({
  open,
  watcher,
  onClose,
  onCreated,
}: {
  open: boolean;
  watcher?: Watcher;
  onClose: () => void;
  onCreated?: (w: Watcher) => void;
}) {
  const t = useT();
  const editing = !!watcher;
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [goal, setGoal] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [every, setEvery] = useState(60);
  const [keepalive, setKeepalive] = useState(0);
  const [mode, setMode] = useState<"replay" | "ai">("replay");
  const [hosts, setHosts] = useState("");
  const [proxy, setProxy] = useState("");
  const [more, setMore] = useState(false);
  const create = useCreateWatcher();
  const update = useUpdateWatcher();
  const pending = create.isPending || update.isPending;

  useEffect(() => {
    if (!open) return;
    setName(watcher?.name ?? "");
    setUrl(watcher?.url ?? "");
    setGoal(watcher?.goal ?? "");
    setUsername("");
    setPassword("");
    setEvery(watcher?.intervalMinutes ?? 60);
    setKeepalive(watcher?.keepaliveMinutes ?? 0);
    setMode(watcher?.mode ?? "replay");
    setHosts(watcher?.allowedHosts.join(", ") ?? "");
    setProxy("");
    setMore(false);
  }, [open, watcher]);

  const valid =
    name.trim() !== "" &&
    /^https?:\/\/\S+$/.test(url.trim()) &&
    goal.trim() !== "";

  const submit = () => {
    const fail = (e: unknown) =>
      toast({ message: errorMessage(e), tone: "error" });
    const hostList = parseHosts(hosts);
    if (watcher) {
      const body: WatcherPatch = {
        name: name.trim(),
        url: url.trim(),
        goal: goal.trim(),
        intervalMinutes: every,
        keepaliveMinutes: keepalive,
        mode,
      };
      if (hostList.join(",") !== watcher.allowedHosts.join(","))
        body.allowedHosts = hostList;
      if (username.trim()) body.username = username.trim();
      if (password) body.password = password;
      if (proxy.trim()) body.proxy = proxy.trim();
      update.mutate(
        { id: watcher.id, body },
        {
          onSuccess: () => {
            toast(t("Saved"));
            onClose();
          },
          onError: fail,
        },
      );
      return;
    }
    const body: WatcherInput = {
      name: name.trim(),
      url: url.trim(),
      goal: goal.trim(),
      intervalMinutes: every,
      keepaliveMinutes: keepalive,
      mode,
    };
    if (hostList.length) body.allowedHosts = hostList;
    if (username.trim()) body.username = username.trim();
    if (password) body.password = password;
    if (proxy.trim()) body.proxy = proxy.trim();
    create.mutate(body, {
      onSuccess: (w) => {
        toast("已建好，AI 开始摸索");
        onClose();
        onCreated?.(w);
      },
      onError: fail,
    });
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      wide
      title={editing ? t("Edit watcher") : t("New watcher")}
      description={
        editing
          ? undefined
          : "给一个网址和一句话目标，AI 会自己找到要盯的内容，记下步骤，以后按步骤定时去看。"
      }
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
        className="watchers-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (valid && !pending) submit();
        }}
      >
        <div className="xc-field">
          <label htmlFor="watcher-name">{t("Name")}</label>
          <input
            id="watcher-name"
            className="xc-input"
            value={name}
            maxLength={100}
            placeholder="比如：签证办理进度"
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="xc-field">
          <label htmlFor="watcher-url">{t("Address")}</label>
          <input
            id="watcher-url"
            className="xc-input"
            value={url}
            inputMode="url"
            placeholder="https://"
            onChange={(e) => setUrl(e.target.value)}
          />
        </div>
        <div className="xc-field full">
          <label htmlFor="watcher-goal">{t("What to watch")}</label>
          <input
            id="watcher-goal"
            className="xc-input"
            value={goal}
            maxLength={1000}
            placeholder="比如：登录后看申请编号 A123 的办理状态"
            onChange={(e) => setGoal(e.target.value)}
          />
          <small>写清楚在哪、看什么。AI 按这句话找。</small>
        </div>
        <div className="xc-field">
          <label htmlFor="watcher-user">{t("Username")}</label>
          <input
            id="watcher-user"
            className="xc-input"
            value={username}
            autoComplete="off"
            placeholder={
              watcher?.hasLogin ? "已存，不改就留空" : "不用登录就留空"
            }
            onChange={(e) => setUsername(e.target.value)}
          />
        </div>
        <div className="xc-field">
          <label htmlFor="watcher-pass">{t("Password")}</label>
          <input
            id="watcher-pass"
            className="xc-input"
            type="password"
            value={password}
            autoComplete="new-password"
            placeholder={watcher?.hasLogin ? "已存，不改就留空" : ""}
            onChange={(e) => setPassword(e.target.value)}
          />
          <small>加密保存，不会发给 AI。</small>
        </div>
        <div className="xc-field">
          <label htmlFor="watcher-every">{t("Check every")}</label>
          <select
            id="watcher-every"
            className="xc-select"
            value={every}
            onChange={(e) => setEvery(Number(e.target.value))}
          >
            {INTERVALS.map((m) => (
              <option key={m} value={m}>
                {minutesText(m)}
              </option>
            ))}
          </select>
        </div>
        <div className="xc-field">
          <label htmlFor="watcher-mode">{t("How AI watches")}</label>
          <select
            id="watcher-mode"
            className="xc-select"
            value={mode}
            onChange={(e) => setMode(e.target.value as "replay" | "ai")}
          >
            <option value="replay">AI 记下步骤，平时回放（省钱）</option>
            <option value="ai">每次都让 AI 看（费钱）</option>
          </select>
        </div>
        {!more ? (
          <button
            type="button"
            className="xc-btn ghost small full"
            onClick={() => setMore(true)}
          >
            {t("More options")}
          </button>
        ) : (
          <>
            <div className="xc-field">
              <label htmlFor="watcher-keepalive">{t("Keep login alive")}</label>
              <select
                id="watcher-keepalive"
                className="xc-select"
                value={keepalive}
                onChange={(e) => setKeepalive(Number(e.target.value))}
              >
                {KEEPALIVES.map((m) => (
                  <option key={m} value={m}>
                    {m === 0 ? "不保活" : `每 ${minutesText(m)}访问一次`}
                  </option>
                ))}
              </select>
              <small>网站有操作就续期时才有用。</small>
            </div>
            <div className="xc-field">
              <label htmlFor="watcher-proxy">{t("Outgoing proxy")}</label>
              <input
                id="watcher-proxy"
                className="xc-input"
                value={proxy}
                autoComplete="off"
                placeholder={
                  watcher?.hasProxy ? "已存，不改就留空" : "socks5://主机:端口"
                }
                onChange={(e) => setProxy(e.target.value)}
              />
              <small>国内网站拦境外 IP 时，填一个国内的代理。</small>
            </div>
            <div className="xc-field full">
              <label htmlFor="watcher-hosts">{t("Allowed domains")}</label>
              <input
                id="watcher-hosts"
                className="xc-input"
                value={hosts}
                placeholder="不填就用网址的域名"
                onChange={(e) => setHosts(e.target.value)}
              />
              <small>
                浏览器只能去这些域名。登录跳到统一认证网站时，把那个域名也加上。
              </small>
            </div>
          </>
        )}
      </form>
    </Dialog>
  );
}

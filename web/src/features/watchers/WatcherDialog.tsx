import { useEffect, useState } from "react";
import { errorMessage } from "../../api/client";
import { useAgents } from "../../api/core";
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
import {
  EGRESS_DIRECT,
  EGRESS_PROXY,
  INTERVALS,
  KEEPALIVES,
  egressAgents,
  egressOf,
  guessKind,
  minutesText,
  parseHosts,
  windowError,
} from "./lib";
import WindowFields from "./WindowFields";

/** 新建或修改一个网站自动化任务。传了 watcher 就是修改。 */
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
  // 用户自己选过间隔就不再跟着类型换默认值
  const [everyTouched, setEveryTouched] = useState(false);
  const [keepalive, setKeepalive] = useState(0);
  const [mode, setMode] = useState<"replay" | "ai">("replay");
  const [hosts, setHosts] = useState("");
  const [proxy, setProxy] = useState("");
  // B208：服务器直接访问、经过一台已连接的机器，或者填代理地址
  const [egress, setEgress] = useState(EGRESS_DIRECT);
  const [more, setMore] = useState(false);
  // B236：检查时段
  const [activeFrom, setActiveFrom] = useState("");
  const [activeTo, setActiveTo] = useState("");
  const [activeDays, setActiveDays] = useState<number[]>([]);
  const agents = useAgents();
  const exits = egressAgents(agents.data ?? [], watcher?.egressAgentId);
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
    setEveryTouched(false);
    setKeepalive(watcher?.keepaliveMinutes ?? 0);
    setMode(watcher?.mode ?? "replay");
    setHosts(watcher?.allowedHosts.join(", ") ?? "");
    setProxy("");
    setEgress(watcher ? egressOf(watcher) : EGRESS_DIRECT);
    setMore(false);
    setActiveFrom(watcher?.activeFrom ?? "");
    setActiveTo(watcher?.activeTo ?? "");
    setActiveDays(watcher?.activeDays ?? []);
  }, [open, watcher]);

  // B213：新建时按网址认类型，修改时类型不变
  const guessed = guessKind(url);
  const kind = watcher?.kind ?? guessed?.kind ?? "web";
  const isX = kind === "x";
  const defaultEvery = isX ? 30 : 60;
  useEffect(() => {
    if (!editing && !everyTouched) setEvery(defaultEvery);
  }, [editing, everyTouched, defaultEvery]);
  const pickEvery = (m: number) => {
    setEvery(m);
    setEveryTouched(true);
  };
  const urlHint = editing
    ? undefined
    : !url.trim()
      ? "网页写完整网址。X 写 @账号、账号或推文的链接，或者搜索词。"
      : guessed
        ? `认出来是：${guessed.label}`
        : "网址要以 https:// 开头";
  const windowOk = windowError(activeFrom, activeTo) === "";
  const valid =
    windowOk &&
    (isX
      ? name.trim() !== "" && url.trim() !== ""
      : name.trim() !== "" &&
        /^https?:\/\/\S+$/.test(url.trim()) &&
        (egress !== EGRESS_PROXY ||
          proxy.trim() !== "" ||
          !!watcher?.hasProxy));
  // 七天都选等于不限；改任务时一直带上三项，清除时段才能传空值
  const windowBody = {
    activeFrom,
    activeTo,
    activeDays: activeDays.length === 7 ? [] : activeDays,
  };
  const windowSet =
    activeFrom !== "" || activeTo !== "" || activeDays.length > 0;
  const viaAgent = egress !== EGRESS_DIRECT && egress !== EGRESS_PROXY;

  const submit = () => {
    const fail = (e: unknown) =>
      toast({ message: errorMessage(e), tone: "error" });
    const hostList = parseHosts(hosts);
    if (watcher && isX) {
      update.mutate(
        {
          id: watcher.id,
          body: {
            name: name.trim(),
            url: url.trim(),
            goal: goal.trim(),
            intervalMinutes: every,
            ...windowBody,
          },
        },
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
    if (isX) {
      create.mutate(
        {
          name: name.trim(),
          url: url.trim(),
          goal: goal.trim(),
          intervalMinutes: every,
          ...(windowSet ? windowBody : {}),
        },
        {
          onSuccess: (w) => {
            toast("已建好，先记下现有的推文");
            onClose();
            onCreated?.(w);
          },
          onError: fail,
        },
      );
      return;
    }
    if (watcher) {
      const body: WatcherPatch = {
        name: name.trim(),
        url: url.trim(),
        goal: goal.trim(),
        intervalMinutes: every,
        keepaliveMinutes: keepalive,
        mode,
        ...windowBody,
      };
      if (hostList.join(",") !== watcher.allowedHosts.join(","))
        body.allowedHosts = hostList;
      if (username.trim()) body.username = username.trim();
      if (password) body.password = password;
      if (egress === EGRESS_PROXY) {
        if (proxy.trim()) body.proxy = proxy.trim();
      } else if (viaAgent) {
        if (egress !== watcher.egressAgentId) body.egressAgentId = egress;
      } else {
        if (watcher.egressAgentId) body.egressAgentId = "";
        if (watcher.hasProxy) body.proxy = "";
      }
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
    // B213：新建时不传类型，后端按网址认（和 guessKind 的规则一样）
    const body: WatcherInput = {
      name: name.trim(),
      url: url.trim(),
      goal: goal.trim(),
      intervalMinutes: every,
      keepaliveMinutes: keepalive,
      mode,
      ...(windowSet ? windowBody : {}),
    };
    if (hostList.length) body.allowedHosts = hostList;
    if (username.trim()) body.username = username.trim();
    if (password) body.password = password;
    if (egress === EGRESS_PROXY && proxy.trim()) body.proxy = proxy.trim();
    if (viaAgent) body.egressAgentId = egress;
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
          : isX
            ? "盯一个账号的新推文、一个搜索的新结果，或者一条推文的新回复。用稍后读里存的 X Cookie。"
            : "给一个网址，AI 会自己找到页面上要盯的内容，记下步骤，以后按步骤定时去看。也可以让它照你说的去操作。"
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
            placeholder={isX ? "比如：美联储消息" : "比如：签证办理进度"}
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
            placeholder="https:// 或者 @账号"
            onChange={(e) => setUrl(e.target.value)}
          />
          {urlHint && <small>{urlHint}</small>}
        </div>
        {isX ? (
          <>
            <XFields
              goal={goal}
              setGoal={setGoal}
              every={every}
              setEvery={pickEvery}
            />
            <WindowFields
              from={activeFrom}
              to={activeTo}
              days={activeDays}
              onChange={(v) => {
                setActiveFrom(v.from);
                setActiveTo(v.to);
                setActiveDays(v.days);
              }}
            />
          </>
        ) : (
          <>
            <div className="xc-field full">
              <label htmlFor="watcher-goal">{t("What AI should do")}</label>
              <input
                id="watcher-goal"
                className="xc-input"
                value={goal}
                maxLength={1000}
                placeholder="可以不填。比如：登录后看申请编号 A123 的办理状态"
                onChange={(e) => setGoal(e.target.value)}
              />
              <small>
                不填就让 AI
                自己找页面上会变的内容，比如状态、进度、余额。填了就照着做，要点提交、删除这类按钮时先问你。
              </small>
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
                onChange={(e) => pickEvery(Number(e.target.value))}
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
            <WindowFields
              from={activeFrom}
              to={activeTo}
              days={activeDays}
              onChange={(v) => {
                setActiveFrom(v.from);
                setActiveTo(v.to);
                setActiveDays(v.days);
              }}
            />
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
                  <label htmlFor="watcher-keepalive">
                    {t("Keep login alive")}
                  </label>
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
                  <label htmlFor="watcher-egress">{t("Go online via")}</label>
                  <select
                    id="watcher-egress"
                    className="xc-select"
                    value={egress}
                    onChange={(e) => setEgress(e.target.value)}
                  >
                    <option value={EGRESS_DIRECT}>服务器直接访问</option>
                    {exits.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.online ? a.name : `${a.name}（离线）`}
                      </option>
                    ))}
                    <option value={EGRESS_PROXY}>填代理地址</option>
                  </select>
                  <small>
                    国内网站拦境外 IP
                    时，选一台在国内的机器，或者填一个国内的代理。
                  </small>
                </div>
                {egress === EGRESS_PROXY && (
                  <div className="xc-field">
                    <label htmlFor="watcher-proxy">{t("Outgoing proxy")}</label>
                    <input
                      id="watcher-proxy"
                      className="xc-input"
                      value={proxy}
                      autoComplete="off"
                      placeholder={
                        watcher?.hasProxy
                          ? "已存，不改就留空"
                          : "socks5://主机:端口"
                      }
                      onChange={(e) => setProxy(e.target.value)}
                    />
                  </div>
                )}
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
          </>
        )}
      </form>
    </Dialog>
  );
}

function XFields(p: {
  goal: string;
  setGoal: (v: string) => void;
  every: number;
  setEvery: (v: number) => void;
}) {
  const t = useT();
  return (
    <>
      <div className="xc-field">
        <label htmlFor="watcher-goal">{t("Only tell me about")}</label>
        <input
          id="watcher-goal"
          className="xc-input"
          value={p.goal}
          maxLength={1000}
          placeholder="可以不填。比如：只要和降息有关的"
          onChange={(e) => p.setGoal(e.target.value)}
        />
        <small>填了就让 AI 先挑一遍，不相关的不提醒。</small>
      </div>
      <div className="xc-field">
        <label htmlFor="watcher-every">{t("Check every")}</label>
        <select
          id="watcher-every"
          className="xc-select"
          value={p.every}
          onChange={(e) => p.setEvery(Number(e.target.value))}
        >
          {INTERVALS.map((m) => (
            <option key={m} value={m}>
              {minutesText(m)}
            </option>
          ))}
        </select>
      </div>
    </>
  );
}

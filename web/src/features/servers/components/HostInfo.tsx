import { useEffect, useState, type FormEvent } from "react";
import { Copy, Eye, EyeOff, Pencil } from "lucide-react";
import { errorMessage } from "../../../api/client";
import Markdown from "../../../components/markdown/Markdown";
import MarkdownEditor from "../../../components/markdown/MarkdownEditor";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  fetchHostPassword,
  usePatchHost,
  type HostDetail,
  type HostInfo,
  type HostInfoInput,
} from "../api";

/*
 * 机器信息（B82）：归属、客户、登录用户名和密码、备注、标签。
 * 编辑弹窗、添加机器时的“更多信息”、详情页的信息卡片都在这里。
 */

export interface InfoDraft {
  ownership: "own" | "client";
  client: string;
  username: string;
  /** 新密码。留空表示不改。 */
  password: string;
  clearPassword: boolean;
  note: string;
  /** 逗号分开 */
  tags: string;
}

export function infoDraft(info?: HostInfo): InfoDraft {
  return {
    ownership: info?.ownership ?? "own",
    client: info?.client ?? "",
    username: info?.username ?? "",
    password: "",
    clearPassword: false,
    note: info?.note ?? "",
    tags: (info?.tags ?? []).join(", "),
  };
}

/** 中英文逗号、顿号都算分隔。 */
export function splitTags(text: string): string[] {
  const out: string[] = [];
  for (const raw of text.split(/[,，、]/)) {
    const tag = raw.trim();
    if (tag && !out.includes(tag)) out.push(tag);
  }
  return out;
}

export function infoInput(d: InfoDraft): HostInfoInput {
  const input: HostInfoInput = {
    ownership: d.ownership,
    client: d.ownership === "client" ? d.client.trim() : "",
    username: d.username.trim(),
    note: d.note,
    tags: splitTags(d.tags),
  };
  if (d.password) input.password = d.password;
  else if (d.clearPassword) input.clearPassword = true;
  return input;
}

/** 填过任何一项吗。添加机器时没填就不传 info。 */
export function infoFilled(d: InfoDraft): boolean {
  return (
    d.ownership === "client" ||
    !!d.username.trim() ||
    !!d.password ||
    !!d.note.trim() ||
    splitTags(d.tags).length > 0
  );
}

export function HostInfoFields({
  draft,
  onChange,
  hasPassword,
}: {
  draft: InfoDraft;
  onChange: (d: InfoDraft) => void;
  hasPassword?: boolean;
}) {
  const t = useT();
  const set = (patch: Partial<InfoDraft>) => onChange({ ...draft, ...patch });
  return (
    <div className="xc-stack servers-info-fields">
      <div className="xc-field">
        <span>{t("Ownership")}</span>
        <div className="xc-row" role="radiogroup" aria-label={t("Ownership")}>
          <label className="xc-check">
            <input
              type="radio"
              name="ownership"
              checked={draft.ownership === "own"}
              onChange={() => set({ ownership: "own" })}
            />
            {t("My own machine")}
          </label>
          <label className="xc-check">
            <input
              type="radio"
              name="ownership"
              checked={draft.ownership === "client"}
              onChange={() => set({ ownership: "client" })}
            />
            {t("A client's")}
          </label>
        </div>
      </div>
      {draft.ownership === "client" && (
        <label className="xc-field">
          <span>{t("Client name")}</span>
          <input
            className="xc-input"
            value={draft.client}
            maxLength={100}
            onChange={(e) => set({ client: e.target.value })}
          />
        </label>
      )}
      <div className="servers-info-pair">
        <label className="xc-field">
          <span>{t("Login username")}</span>
          <input
            className="xc-input"
            value={draft.username}
            autoComplete="off"
            maxLength={200}
            onChange={(e) => set({ username: e.target.value })}
          />
        </label>
        <label className="xc-field">
          <span>{t("Login password")}</span>
          <input
            className="xc-input"
            type="password"
            value={draft.password}
            autoComplete="new-password"
            maxLength={500}
            placeholder={
              hasPassword && !draft.clearPassword
                ? t("Leave empty to keep the old password")
                : ""
            }
            onChange={(e) =>
              set({ password: e.target.value, clearPassword: false })
            }
          />
          <small>{t("Stored encrypted. Showing it needs verification.")}</small>
        </label>
      </div>
      {hasPassword && !draft.password && (
        <label className="xc-check">
          <input
            type="checkbox"
            checked={draft.clearPassword}
            onChange={(e) => set({ clearPassword: e.target.checked })}
          />
          {t("Remove the saved password")}
        </label>
      )}
      <div className="xc-field">
        <span>{t("Host notes")}</span>
        <MarkdownEditor
          value={draft.note}
          onChange={(note) => set({ note })}
          label={t("Host notes")}
          minRows={3}
          placeholder={t("Things to watch out for on this machine")}
        />
      </div>
      <label className="xc-field">
        <span>{t("Tags")}</span>
        <input
          className="xc-input"
          value={draft.tags}
          placeholder={t("Comma separated, e.g. production, Hong Kong")}
          onChange={(e) => set({ tags: e.target.value })}
        />
      </label>
    </div>
  );
}

/** “编辑信息”弹窗：名称和信息。 */
export function HostInfoDialog({
  host,
  onClose,
}: {
  host: Pick<HostDetail, "id" | "name" | "info">;
  onClose: () => void;
}) {
  const t = useT();
  const patch = usePatchHost(host.id);
  const [name, setName] = useState(host.name);
  const [draft, setDraft] = useState(() => infoDraft(host.info));
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    patch.mutate(
      { name: name.trim(), info: infoInput(draft) },
      {
        onSuccess: () => {
          toast(t("Saved"));
          onClose();
        },
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  };
  return (
    <Dialog
      open
      onClose={onClose}
      title={t("Edit info")}
      description={host.name}
    >
      <form className="xc-stack servers-info-form" onSubmit={submit}>
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            required
            maxLength={100}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <HostInfoFields
          draft={draft}
          onChange={setDraft}
          hasPassword={host.info.hasPassword}
        />
        <div className="xc-dialog-actions">
          <button type="button" className="xc-btn ghost" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            type="submit"
            className="xc-btn primary"
            disabled={patch.isPending || !name.trim()}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** 密码：圆点，“显示”要验证，30 秒后自动收起。 */
function PasswordValue({ hostId }: { hostId: string }) {
  const t = useT();
  const [shown, setShown] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (shown == null) return;
    const timer = window.setTimeout(() => setShown(null), 30_000);
    return () => window.clearTimeout(timer);
  }, [shown]);
  const reveal = async () => {
    setBusy(true);
    try {
      setShown(await fetchHostPassword(hostId));
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    } finally {
      setBusy(false);
    }
  };
  const copy = async () => {
    try {
      const value = shown ?? (await fetchHostPassword(hostId));
      await navigator.clipboard.writeText(value);
      toast(t("Copied"));
    } catch (e) {
      toast({ message: errorMessage(e), tone: "error" });
    }
  };
  return (
    <span className="servers-info-secret">
      <span className="xc-mono">{shown ?? "••••••••"}</span>
      <button
        type="button"
        className="xc-btn ghost small"
        disabled={busy}
        title={shown == null ? t("Show") : t("Hide")}
        aria-label={shown == null ? t("Show password") : t("Hide password")}
        onClick={() => (shown == null ? void reveal() : setShown(null))}
      >
        {shown == null ? <Eye size={14} /> : <EyeOff size={14} />}
      </button>
      <button
        type="button"
        className="xc-btn ghost small"
        title={t("Copy")}
        aria-label={t("Copy password")}
        onClick={() => void copy()}
      >
        <Copy size={14} />
      </button>
    </span>
  );
}

/** 详情页概览里的“信息”卡片。 */
export function HostInfoCard({ host }: { host: HostDetail }) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const info = host.info;
  const empty =
    info.ownership === "own" &&
    !info.username &&
    !info.hasPassword &&
    !info.note &&
    info.tags.length === 0;
  return (
    <section className="xc-card servers-info-card">
      <div className="xc-card-head">
        <h3>{t("Machine info")}</h3>
        {!empty && (
          <button
            type="button"
            className="xc-btn ghost small"
            onClick={() => setEditing(true)}
          >
            <Pencil size={13} /> {t("Edit")}
          </button>
        )}
      </div>
      {empty ? (
        <div className="servers-info-empty">
          <span className="xc-muted">{t("No info yet")}</span>
          <button
            type="button"
            className="xc-btn small"
            onClick={() => setEditing(true)}
          >
            {t("Fill in")}
          </button>
        </div>
      ) : (
        <dl className="servers-info-dl">
          <dt>{t("Ownership")}</dt>
          <dd>
            {info.ownership === "client"
              ? `${t("A client's")}${info.client ? ` · ${info.client}` : ""}`
              : t("My own machine")}
          </dd>
          {info.username && (
            <>
              <dt>{t("Login username")}</dt>
              <dd className="xc-mono">{info.username}</dd>
            </>
          )}
          {info.hasPassword && (
            <>
              <dt>{t("Login password")}</dt>
              <dd>
                <PasswordValue hostId={host.id} />
              </dd>
            </>
          )}
          {info.tags.length > 0 && (
            <>
              <dt>{t("Tags")}</dt>
              <dd className="xc-row">
                {info.tags.map((tag) => (
                  <span key={tag} className="xc-badge">
                    {tag}
                  </span>
                ))}
              </dd>
            </>
          )}
          {info.note && (
            <>
              <dt>{t("Host notes")}</dt>
              <dd className="servers-info-note">
                <Markdown source={info.note} />
              </dd>
            </>
          )}
        </dl>
      )}
      {editing && (
        <HostInfoDialog host={host} onClose={() => setEditing(false)} />
      )}
    </section>
  );
}

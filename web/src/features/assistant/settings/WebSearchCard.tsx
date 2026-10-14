import { useEffect, useState } from "react";
import { ExternalLink } from "lucide-react";
import { errorMessage } from "../../../api/client";
import { Loading } from "../../../components/ui/States";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import {
  isNotLive,
  useSaveWebSearch,
  useTestWebSearch,
  useWebSearch,
  type AiWebSearchMode,
} from "../api";

const MODES: { value: AiWebSearchMode; label: string }[] = [
  { value: "off", label: "Web search off" },
  { value: "openai", label: "OpenAI built-in search" },
  { value: "brave", label: "Brave Search" },
];

/**
 * 设置 → AI 的“联网搜索”（B211）。菜谱的“找新菜谱”用它，以后别的功能也能用。
 * OpenAI 自带的搜索要用 Responses 接口的服务商；Brave 要填密钥。
 */
export default function WebSearchCard() {
  const t = useT();
  const current = useWebSearch();
  const save = useSaveWebSearch();
  const test = useTestWebSearch();
  const [mode, setMode] = useState<AiWebSearchMode>("off");
  const [key, setKey] = useState("");

  useEffect(() => {
    if (current.data) setMode(current.data.mode);
  }, [current.data]);

  const onSave = () =>
    save.mutate(key.trim() ? { mode, braveKey: key.trim() } : { mode }, {
      onSuccess: () => {
        setKey("");
        test.reset();
        toast(t("Saved"));
      },
      onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
    });

  let body;
  if (current.isPending) body = <Loading />;
  else if (current.isError)
    body = (
      <p className="xc-muted">
        {isNotLive(current.error)
          ? t("Not live yet")
          : errorMessage(current.error)}
      </p>
    );
  else
    body = (
      <>
        <label className="xc-field">
          <span>{t("Search with")}</span>
          <select
            className="xc-select"
            value={mode}
            onChange={(e) => setMode(e.target.value as AiWebSearchMode)}
          >
            {MODES.map((m) => (
              <option key={m.value} value={m.value}>
                {t(m.label)}
              </option>
            ))}
          </select>
          {mode === "openai" && (
            <small>
              {t(
                "Uses the main model. Its provider must use the Responses API, such as OpenAI itself.",
              )}
            </small>
          )}
        </label>
        {mode === "brave" && (
          <label className="xc-field">
            <span>{t("Brave Search API key")}</span>
            <input
              className="xc-input"
              type="password"
              autoComplete="off"
              value={key}
              placeholder={
                current.data.braveKeySet
                  ? t("Saved. Type a new key to replace it")
                  : t("Paste the key from api.search.brave.com")
              }
              onChange={(e) => setKey(e.target.value)}
            />
            <small>
              {t(
                "Pages found are read on the server. Addresses on this machine or the local network are refused.",
              )}
            </small>
          </label>
        )}
        <p className="xc-muted">
          {t(
            "Each search uses AI credit, and Brave may charge for searches too. Only used when you ask, never on a schedule.",
          )}
        </p>
        {test.isError && (
          <p className="ai-web-error">{errorMessage(test.error)}</p>
        )}
        {test.isSuccess && (
          <ul className="xc-list ai-web-results">
            {test.data.sources.length === 0 && (
              <li className="xc-muted">{t("No pages found")}</li>
            )}
            {test.data.sources.map((s) => (
              <li key={s.url}>
                <a href={s.url} target="_blank" rel="noreferrer">
                  <ExternalLink size={13} /> {s.title || s.url}
                </a>
              </li>
            ))}
          </ul>
        )}
        <div className="xc-row ai-web-actions">
          <button
            className="xc-btn"
            disabled={
              test.isPending ||
              current.data.mode === "off" ||
              mode !== current.data.mode
            }
            title={
              mode !== current.data.mode
                ? t("Save first, then test")
                : undefined
            }
            onClick={() => test.mutate()}
          >
            {test.isPending ? t("Searching…") : t("Test")}
          </button>
          <button
            className="xc-btn primary"
            disabled={save.isPending}
            onClick={onSave}
          >
            {t("Save")}
          </button>
        </div>
      </>
    );

  return (
    <section className="xc-card ai-web-search" id="ai-web-search">
      <div className="xc-card-head">
        <h2>{t("Web search")}</h2>
      </div>
      {body}
    </section>
  );
}

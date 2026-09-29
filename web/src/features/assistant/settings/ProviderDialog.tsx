import { useEffect, useState, type FormEvent } from "react";
import { errorMessage } from "../../../api/client";
import Dialog from "../../../components/ui/Dialog";
import { useT } from "../../../contexts/LanguageContext";
import { toast } from "../../../hooks/useToast";
import { useProviderMutations, type AiProvider } from "../api";
import { PROVIDER_PRESETS } from "./models";

/** 新建或编辑一个 OpenAI 兼容接口的供应商。 */
export default function ProviderDialog({
  open,
  onClose,
  provider,
}: {
  open: boolean;
  onClose: () => void;
  /** 不传表示新建 */
  provider?: AiProvider;
}) {
  const t = useT();
  const ops = useProviderMutations();
  const [name, setName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setName(provider?.name ?? "");
    setBaseUrl(provider?.baseUrl ?? "");
    setApiKey("");
    setError("");
  }, [open, provider]);

  const pending = ops.create.isPending || ops.update.isPending;
  const validUrl = /^https?:\/\/\S+$/.test(baseUrl.trim());

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !validUrl) return;
    setError("");
    const done = {
      onSuccess: () => {
        toast(t("Saved"));
        onClose();
      },
      onError: (err: unknown) => setError(errorMessage(err)),
    };
    if (provider)
      ops.update.mutate(
        {
          id: provider.id,
          body: {
            name: name.trim(),
            baseUrl: baseUrl.trim(),
            apiKey: apiKey.trim() || undefined,
          },
        },
        done,
      );
    else
      ops.create.mutate(
        {
          name: name.trim(),
          baseUrl: baseUrl.trim(),
          apiKey: apiKey.trim() || undefined,
        },
        done,
      );
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={provider ? t("Edit provider") : t("Add provider")}
      description={t(
        "Any service with an OpenAI compatible API works, including local models.",
      )}
    >
      <form onSubmit={submit}>
        {!provider && (
          <div className="ai-presets" role="group" aria-label={t("Presets")}>
            {PROVIDER_PRESETS.map((p) => (
              <button
                key={p.name}
                type="button"
                className={`xc-btn small${baseUrl === p.baseUrl ? " primary" : ""}`}
                onClick={() => {
                  setName(p.name);
                  setBaseUrl(p.baseUrl);
                }}
              >
                {p.name}
              </button>
            ))}
          </div>
        )}
        <label className="xc-field">
          <span>{t("Name")}</span>
          <input
            className="xc-input"
            value={name}
            maxLength={40}
            onChange={(e) => setName(e.target.value)}
            placeholder="DeepSeek"
            required
          />
        </label>
        <label className="xc-field">
          <span>Base URL</span>
          <input
            className="xc-input xc-mono"
            value={baseUrl}
            onChange={(e) => setBaseUrl(e.target.value)}
            placeholder="https://api.example.com/v1"
            required
          />
          {baseUrl.trim() && !validUrl && (
            <small className="xc-error-text">
              {t("Starts with http:// or https://")}
            </small>
          )}
        </label>
        <label className="xc-field">
          <span>API Key</span>
          <input
            className="xc-input"
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder={
              provider?.hasApiKey ? t("leave empty to keep") : "sk-..."
            }
            autoComplete="new-password"
          />
          <small>
            {t("Stored encrypted on the server. Local models need no key.")}
          </small>
        </label>
        {error && <p className="xc-error-text">{error}</p>}
        <div className="xc-dialog-actions">
          {provider?.hasApiKey && (
            <button
              type="button"
              className="xc-btn ghost"
              disabled={pending}
              onClick={() =>
                ops.update.mutate(
                  { id: provider.id, body: { apiKey: "" } },
                  {
                    onSuccess: () => {
                      toast(t("API key removed"));
                      onClose();
                    },
                    onError: (err) => setError(errorMessage(err)),
                  },
                )
              }
            >
              {t("Remove API key")}
            </button>
          )}
          <span className="xc-spacer" />
          <button type="button" className="xc-btn" onClick={onClose}>
            {t("Cancel")}
          </button>
          <button
            className="xc-btn primary"
            disabled={pending || !name.trim() || !validUrl}
          >
            {t("Save")}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

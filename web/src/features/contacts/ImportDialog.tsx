import { useRef, useState } from "react";
import { Cloud, RefreshCw, Upload } from "lucide-react";
import { ApiError, errorMessage } from "../../api/client";
import Dialog from "../../components/ui/Dialog";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useContactSync,
  useDeleteContactSync,
  useImportContacts,
  useRunContactSync,
  useSetContactSync,
} from "./api";

/** 在这个弹窗里导入 vCard 文件，或者连上 iCloud 通讯录自动同步。 */
export default function ImportDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const language = useLanguage();
  const fileInput = useRef<HTMLInputElement>(null);
  const importFile = useImportContacts();
  const [onlyDates, setOnlyDates] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const sync = useContactSync();
  const setSync = useSetContactSync();
  const stopSync = useDeleteContactSync();
  const runSync = useRunContactSync();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [syncOnlyDates, setSyncOnlyDates] = useState(false);
  const [syncError, setSyncError] = useState("");

  const status = sync.data;
  const canceled = (err: unknown) =>
    err instanceof ApiError && err.code === "elevation_canceled";

  const pick = async (file: File | undefined) => {
    if (!file) return;
    setError("");
    setMessage("");
    try {
      const r = await importFile.mutateAsync({
        file,
        onlyWithDates: onlyDates,
      });
      setMessage(
        `${t("Import finished")}：${t("new")} ${r.created}，${t("updated")} ${r.updated}，${t("skipped")} ${r.skipped}`,
      );
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      if (fileInput.current) fileInput.current.value = "";
    }
  };

  const connect = async () => {
    setSyncError("");
    try {
      const s = await setSync.mutateAsync({
        username: username.trim(),
        password: password.trim(),
        onlyWithDates: syncOnlyDates,
      });
      setPassword("");
      toast(
        `${t("Synced")}：${t("new")} ${s.created}，${t("updated")} ${s.updated}`,
      );
    } catch (err) {
      if (!canceled(err)) setSyncError(errorMessage(err));
    }
  };

  const when = (iso: string) =>
    new Date(iso).toLocaleString(language === "zh" ? "zh-CN" : "en", {
      month: "numeric",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });

  return (
    <Dialog open={open} onClose={onClose} title={t("Import and sync")} wide>
      <section className="contacts-import">
        <h3>{t("Import a vCard file")}</h3>
        <p className="xc-muted">
          {t(
            "Google Contacts: open contacts.google.com, choose Export, pick vCard, then choose the file here. iPhone and iCloud contacts export the same way.",
          )}
        </p>
        <label className="contacts-check">
          <input
            type="checkbox"
            checked={onlyDates}
            onChange={(e) => setOnlyDates(e.target.checked)}
          />
          <span>{t("Only people with a birthday or another date")}</span>
        </label>
        <button
          className="xc-btn"
          disabled={importFile.isPending}
          onClick={() => fileInput.current?.click()}
        >
          <Upload size={14} />{" "}
          {importFile.isPending ? t("Importing…") : t("Choose a .vcf file")}
        </button>
        <input
          ref={fileInput}
          type="file"
          accept=".vcf,text/vcard,text/x-vcard"
          hidden
          data-testid="vcf-input"
          onChange={(e) => void pick(e.target.files?.[0])}
        />
        {message && <p className="contacts-import-ok">{message}</p>}
        {error && <p className="xc-error-text">{error}</p>}
        <small className="xc-muted">
          {t(
            "People already here are matched by their ID in the file, or by name. Their group, reminders and notes are kept.",
          )}
        </small>
      </section>

      <section className="contacts-import">
        <h3>
          <Cloud size={15} /> {t("iCloud contacts sync")}
        </h3>
        {status?.configured ? (
          <>
            <p>
              {t("Connected")}：<strong>{status.username}</strong>
            </p>
            <p className="xc-muted">
              {status.lastSyncAt
                ? `${t("Last sync")} ${when(status.lastSyncAt)}，${status.total} ${t("people")}，${t("new")} ${status.created}，${t("updated")} ${status.updated}`
                : t("Not synced yet")}
            </p>
            {status.lastError && (
              <p className="xc-error-text">
                {t("Last sync failed")}：{status.lastError}
              </p>
            )}
            <div className="contacts-import-actions">
              <button
                className="xc-btn"
                disabled={runSync.isPending || status.syncing}
                onClick={() =>
                  runSync.mutate(undefined, {
                    onError: (err) => setSyncError(errorMessage(err)),
                  })
                }
              >
                <RefreshCw size={14} />{" "}
                {runSync.isPending || status.syncing
                  ? t("Syncing…")
                  : t("Sync now")}
              </button>
              <button
                className="xc-btn ghost danger"
                disabled={stopSync.isPending}
                onClick={() =>
                  stopSync.mutate(undefined, {
                    onError: (err) => {
                      if (!canceled(err)) setSyncError(errorMessage(err));
                    },
                  })
                }
              >
                {t("Stop syncing")}
              </button>
            </div>
            <small className="xc-muted">
              {t(
                "It syncs every 6 hours. Contacts only come from iCloud to here. Nothing on iCloud is changed, and nobody is deleted here.",
              )}
            </small>
          </>
        ) : (
          <>
            <p className="xc-muted">
              {t(
                "Use an app-specific password, not your Apple ID password: sign in at appleid.apple.com, open Sign-In and Security, then App-Specific Passwords.",
              )}
            </p>
            <div className="contacts-form-row">
              <label className="xc-field">
                <span>Apple ID</span>
                <input
                  className="xc-input"
                  value={username}
                  autoComplete="off"
                  onChange={(e) => setUsername(e.target.value)}
                />
              </label>
              <label className="xc-field">
                <span>{t("App-specific password")}</span>
                <input
                  className="xc-input"
                  type="password"
                  value={password}
                  autoComplete="new-password"
                  placeholder="xxxx-xxxx-xxxx-xxxx"
                  onChange={(e) => setPassword(e.target.value)}
                />
              </label>
            </div>
            <label className="contacts-check">
              <input
                type="checkbox"
                checked={syncOnlyDates}
                onChange={(e) => setSyncOnlyDates(e.target.checked)}
              />
              <span>{t("Only people with a birthday or another date")}</span>
            </label>
            <button
              className="xc-btn primary"
              disabled={
                setSync.isPending || !username.trim() || !password.trim()
              }
              onClick={() => void connect()}
            >
              {setSync.isPending ? t("Connecting…") : t("Connect and sync")}
            </button>
          </>
        )}
        {syncError && <p className="xc-error-text">{syncError}</p>}
      </section>
      <div className="xc-dialog-actions">
        <button className="xc-btn" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

import { useRef, useState, type FormEvent } from "react";
import { Inbox as InboxIcon, Mic, Settings, Square } from "lucide-react";
import { errorMessage, isNotLive } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import {
  EmptyState,
  ErrorState,
  Loading,
  NotLive,
} from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useAddInbox,
  useAddVoice,
  useClosedInbox,
  useInbox,
  useInboxSettings,
} from "./api";
import InboxSettingsDialog from "./InboxSettingsDialog";
import ItemCard from "./ItemCard";
import { KIND_ZH } from "./format";
import { canRecord, startRecording, type Recording } from "./recorder";
import "./i18n";
import "./inbox.css";

const fail = (err: unknown) =>
  toast({ message: errorMessage(err), tone: "error" });

/** 随手记收集箱（B134）：先收下，再确认归到哪里。 */
export default function InboxPage() {
  const t = useT();
  const list = useInbox();
  const settings = useInboxSettings();
  const add = useAddInbox();
  const voice = useAddVoice();
  const [text, setText] = useState("");
  const [recording, setRecording] = useState(false);
  const [showClosed, setShowClosed] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const closed = useClosedInbox(showClosed);
  const recorder = useRef<Recording | null>(null);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const value = text.trim();
    if (!value) return;
    add.mutate(value, { onSuccess: () => setText(""), onError: fail });
  };

  const toggleRecording = async () => {
    if (recording) {
      const rec = recorder.current;
      recorder.current = null;
      setRecording(false);
      if (!rec) return;
      const blob = await rec.stop();
      if (blob.size === 0) return;
      voice.mutate(
        { blob, language: settings.data?.language || undefined },
        { onError: fail },
      );
      return;
    }
    try {
      recorder.current = await startRecording();
      setRecording(true);
    } catch {
      toast({
        message: t(
          "The microphone is not available. Check the browser permission.",
        ),
        tone: "error",
      });
    }
  };

  let body;
  if (list.isPending) body = <Loading />;
  else if (list.isError && isNotLive(list.error))
    body = <NotLive name={t("Capture inbox")} icon={<InboxIcon size={28} />} />;
  else if (list.isError)
    body = <ErrorState error={list.error} onRetry={() => list.refetch()} />;
  else if (list.data.items.length === 0)
    body = (
      <EmptyState
        title={t("The inbox is empty")}
        icon={<InboxIcon size={28} />}
      >
        <span>
          {t(
            "Write down anything that crosses your mind. You decide later where it goes.",
          )}
        </span>
      </EmptyState>
    );
  else
    body = (
      <div className="inbox-list">
        {list.data.items.map((item) => (
          <ItemCard key={item.id} item={item} settings={settings.data} />
        ))}
      </div>
    );

  const open = list.data?.openCount ?? 0;
  return (
    <div className="xc-page inbox-page">
      <PageHeading
        title={t("Capture inbox")}
        subtitle={open > 0 ? `${open} ${t("Inbox waiting")}` : undefined}
        aside={
          <button
            className="xc-btn small"
            onClick={() => setSettingsOpen(true)}
          >
            <Settings size={14} /> {t("Inbox settings")}
          </button>
        }
      />
      <form className="xc-card inbox-capture" onSubmit={submit}>
        <textarea
          className="xc-textarea"
          rows={2}
          value={text}
          maxLength={5000}
          aria-label={t("Write a note")}
          placeholder={t(
            "Write a note. Enter saves, Shift+Enter starts a new line.",
          )}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (
              e.key === "Enter" &&
              !e.shiftKey &&
              !e.nativeEvent.isComposing
            ) {
              e.preventDefault();
              submit(e);
            }
          }}
        />
        <div className="inbox-capture-actions">
          {canRecord() && (
            <button
              type="button"
              className={`xc-btn small${recording ? " danger" : ""}`}
              aria-pressed={recording}
              disabled={voice.isPending}
              onClick={toggleRecording}
            >
              {recording ? <Square size={14} /> : <Mic size={14} />}{" "}
              {recording
                ? t("Stop and save")
                : voice.isPending
                  ? t("Turning speech into text…")
                  : t("Record")}
            </button>
          )}
          <button
            type="submit"
            className="xc-btn small primary"
            disabled={add.isPending || !text.trim()}
          >
            {t("Save to inbox")}
          </button>
        </div>
      </form>
      {body}
      <details
        className="inbox-closed"
        onToggle={(e) => setShowClosed((e.target as HTMLDetailsElement).open)}
      >
        <summary>{t("Recently handled")}</summary>
        {closed.isPending && showClosed && <Loading />}
        <ul>
          {closed.data?.items.map((item) => (
            <li key={item.id}>
              <span>{item.text.slice(0, 80)}</span>
              <small className="xc-muted">
                {item.status === "filed"
                  ? `${t("Filed as")}${KIND_ZH[item.filedKind ?? "none"]}${item.filedRef ? ` ${item.filedRef}` : ""}`
                  : t("Dismissed")}
              </small>
            </li>
          ))}
        </ul>
        {closed.data && closed.data.items.length === 0 && (
          <p className="xc-muted">{t("Nothing yet.")}</p>
        )}
      </details>
      <InboxSettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        settings={settings.data}
      />
    </div>
  );
}

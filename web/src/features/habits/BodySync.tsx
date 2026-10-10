import { useState } from "react";
import { errorMessage } from "../../api/client";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { ErrorState, Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { copyText } from "../../lib/errors";
import {
  useBodyPush,
  useCreateBodyPushToken,
  useDeleteBodyPush,
  type BodyPushToken,
} from "./personalApi";

/** B119：手机快捷指令、Home Assistant 或脚本每天上报身体数据。 */
export default function BodySync() {
  const t = useT();
  const language = useLanguage();
  const status = useBodyPush();
  const create = useCreateBodyPushToken();
  const remove = useDeleteBodyPush();
  const [issued, setIssued] = useState<BodyPushToken | null>(null);
  const [copied, setCopied] = useState(false);

  if (status.isPending) return <Loading />;
  if (status.isError)
    return <ErrorState error={status.error} onRetry={() => status.refetch()} />;
  const s = status.data;

  const onCreate = async () => {
    if (s.enabled) {
      const ok = await confirmAction({
        title: t("Create a new token?"),
        description: t(
          "The old token stops working at once. Update the shortcut or script with the new one.",
        ),
        confirmLabel: t("Create a new token"),
        danger: false,
      });
      if (!ok) return;
    }
    create.mutate(undefined, {
      onSuccess: (data) => {
        setIssued(data);
        setCopied(false);
        toast(t("Report token created"));
      },
      onError: (error) =>
        toast({ message: errorMessage(error), tone: "error" }),
    });
  };
  const onDelete = async () => {
    const ok = await confirmAction({
      title: t("Turn off automatic sync?"),
      description: t(
        "The token stops working. Records already saved are kept.",
      ),
      confirmLabel: t("Turn off"),
    });
    if (!ok) return;
    remove.mutate(undefined, {
      onSuccess: () => {
        setIssued(null);
        toast(t("Automatic sync turned off"));
      },
      onError: (error) =>
        toast({ message: errorMessage(error), tone: "error" }),
    });
  };
  const when = (iso: string) =>
    new Date(iso).toLocaleString(language === "zh" ? "zh-CN" : undefined);

  return (
    <section className="xc-card habits-body-sync">
      <div className="xc-card-head">
        <h2>{t("Automatic sync")}</h2>
      </div>
      <p className="xc-muted">
        {t(
          "A phone shortcut, Home Assistant or a script sends body data once a day and it is saved to that day's record.",
        )}
      </p>
      <p>
        {!s.enabled
          ? t("Not turned on")
          : s.lastReportAt
            ? `${t("Last report")}：${when(s.lastReportAt)}（${s.lastReportDate}）`
            : t("No report received yet")}
      </p>
      <div className="xc-dialog-actions">
        {s.enabled && (
          <button
            type="button"
            className="xc-btn"
            disabled={remove.isPending}
            onClick={onDelete}
          >
            {t("Turn off")}
          </button>
        )}
        <button
          type="button"
          className="xc-btn primary"
          disabled={create.isPending}
          onClick={onCreate}
        >
          {s.enabled ? t("Create a new token") : t("Create report token")}
        </button>
      </div>
      {issued && (
        <>
          <p>
            {t(
              "The token is shown only once. Copy the command now or put the address and token into the shortcut.",
            )}
          </p>
          <div className="habits-code">
            <button
              type="button"
              className="xc-btn small"
              onClick={async () => {
                if (await copyText(issued.example)) {
                  setCopied(true);
                  window.setTimeout(() => setCopied(false), 2000);
                }
              }}
            >
              {copied ? t("Copied") : t("Copy")}
            </button>
            <pre>{issued.example}</pre>
          </div>
        </>
      )}
      <details className="habits-body-help">
        <summary>{t("How to set it up")}</summary>
        <p>
          {t(
            "Fields: weight (kg), waist (cm), sleep (hours), restingHr (bpm), steps, and date (YYYY-MM-DD, today if left out). Send at least one metric. Fields you leave out are not changed.",
          )}
        </p>
        <p>
          {t(
            "Address: {url}. Header: Authorization: Bearer plus the token. Body: JSON. Numbers can be written as numbers or as text.",
          ).replace("{url}", s.reportUrl)}
        </p>
        <ol>
          <li>
            {t(
              "iPhone: in Shortcuts, read Body Mass, Sleep, Resting Heart Rate and Steps with Find Health Samples.",
            )}
          </li>
          <li>
            {t(
              "Add Get Contents of URL, method POST, with the header and a JSON body.",
            )}
          </li>
          <li>
            {t(
              "In Automation, run the shortcut at a fixed time every day, for example 09:00 for last night's sleep.",
            )}
          </li>
          <li>
            {t(
              "Home Assistant: use a rest_command that POSTs to the same address, and call it from an automation once a day.",
            )}
          </li>
        </ol>
      </details>
    </section>
  );
}

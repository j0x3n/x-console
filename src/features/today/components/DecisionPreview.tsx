import type * as Model from "../../../types/domain";
import React from "react";
import { useT } from "../../../contexts/LanguageContext";
import { FileText, ArrowRight } from "lucide-react";
import LineChart from "../../../components/ui/LineChart";

interface DecisionPreviewProps {
  kind: string;
  openDraft: () => void;
}

export default function DecisionPreview({
  kind,
  openDraft,
}: DecisionPreviewProps) {
  const t = useT();
  if (kind === "draft")
    return (
      <button
        className="decision-preview draft-preview"
        onClick={openDraft}
        title={t("Open the draft")}
      >
        <span className="preview-muted">
          <FileText size={13} /> {t("To Priya Raman")}
        </span>
        <strong>{t("Two pricing options before our 10:00")}</strong>
        <p>
          {t(
            "Hi Priya — thanks for walking us through the fleet telemetry setup yesterday. Jun’s question about tracing handoffs between picking agents is exactly where xcc earns its keep.",
          )}
        </p>
      </button>
    );
  if (kind === "health")
    return (
      <div className="decision-preview health-preview">
        <div className="health-top">
          <span>
            {t("Health")}{" "}
            <strong>
              81 <ArrowRight size={12} /> 63
            </strong>
          </span>
          <span className="negative">−18 {t("in 14d")}</span>
        </div>
        <LineChart
          points="0,8 24,9 48,10 72,12 96,15 120,21 142,26 160,29"
          color="#d9af67"
          fill
        />
        <div className="health-bottom">
          <span>{t("Seats in use")}</span>
          <b>
            118 → 92 <em>−22%</em>
          </b>
        </div>
      </div>
    );
  return (
    <div className="decision-preview stage-preview">
      <div className="stage-line">
        <span>{t("Proposal")}</span>
        <ArrowRight size={13} />
        <b>{t("Negotiation")}</b>
      </div>
      <div className="stage-steps">
        <i />
        <i />
        <i />
        <i />
      </div>
      <div className="stage-labels">
        <span>{t("Discovery")}</span>
        <span>{t("Proposal")}</span>
        <span>{t("Terms")}</span>
        <span>{t("Negotiation")}</span>
      </div>
      <div className="stage-receipt">
        <FileText size={12} /> {t("Legal redlines received · 7:48 AM")}
      </div>
    </div>
  );
}

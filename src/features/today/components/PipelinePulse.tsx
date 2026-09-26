import type * as Model from "../../../types/domain";
import React from "react";
import { useT } from "../../../contexts/LanguageContext";
import MetricCard from "../../../components/ui/MetricCard";
import { ArrowUpRight } from "lucide-react";
import LineChart from "../../../components/ui/LineChart";

interface PipelinePulseProps {
  navigate: Model.Navigate;
}

export default function PipelinePulse({ navigate }: PipelinePulseProps) {
  const t = useT();
  return (
    <section className="metrics" aria-label={t("Pipeline pulse")}>
      <MetricCard
        label={t("Coverage")}
        caption={t("Target 3×")}
        onClick={() => navigate("Deals")}
        className="coverage-card"
      >
        <div className="coverage-gauge">
          <div className="gauge-center">
            3.0<span>×</span>
          </div>
        </div>
        <div className="metric-foot">$4.82M · 38 {t("deals")}</div>
      </MetricCard>
      <MetricCard
        label={t("Win rate")}
        caption={t("90 days")}
        onClick={() => navigate("Deals")}
        className="win-card"
      >
        <div className="win-body">
          <div className="win-ring" />
          <div>
            <strong>27%</strong>
            <span className="trend-up">
              <ArrowUpRight size={11} /> 5 {t("pts")}
            </span>
          </div>
        </div>
        <LineChart
          points="0,29 12,26 25,27 38,23 50,25 63,19 76,21 88,17 101,19 113,15 127,17 141,10 154,11 160,8"
          fill
        />
      </MetricCard>
      <MetricCard
        label={t("Q4 forecast")}
        caption={t("82% of target")}
        onClick={() => navigate("Deals")}
        className="forecast-card"
      >
        <div className="metric-value">
          $1.31M <small>{t("commit")}</small>
        </div>
        <div className="metric-secondary">$1.94M {t("best")}</div>
        <div
          className="forecast-track"
          role="img"
          aria-label={t("Q4 forecast")}
        >
          <i />
          <b />
        </div>
        <div className="axis">
          <span>$0</span>
          <span>$1.60M {t("target")}</span>
        </div>
      </MetricCard>
      <MetricCard
        label={t("First response")}
        caption={t("SLA 4h")}
        onClick={() => navigate("Companies")}
        className="response-card"
      >
        <div className="metric-value">{t("2h 14m")}</div>
        <div className="metric-secondary positive">↓ {t("18m faster")}</div>
        <div className="response-bars">
          {Array.from({ length: 27 }, (_, i) => (
            <i
              key={i}
              style={{
                opacity: i < 12 ? 1 : 0.19,
                background: i < 9 ? "#56c7ad" : i < 15 ? "#e2ba63" : "#a96c80",
              }}
            />
          ))}
        </div>
        <div className="axis">
          <span>0</span>
          <span>{t("1 hour")}</span>
          <span>{t("4 hours")}</span>
          <span>{t("6 hours")}</span>
        </div>
      </MetricCard>
      <MetricCard
        label={t("Crew today")}
        caption={t("37 overnight")}
        onClick={() => navigate("Crew")}
        className="crew-card"
      >
        <div className="metric-value">
          142 <small>{t("runs")}</small>
        </div>
        <div className="budget-bar">
          <i />
          <i />
          <i />
          <i />
          <i />
        </div>
        <div className="metric-foot">$18.40 {t("of $30 budget")}</div>
      </MetricCard>
      <MetricCard
        label={t("Next meeting")}
        caption={t("10:00 AM")}
        onClick={() => navigate("Halcyon Robotics")}
        className="next-card"
      >
        <div className="meeting-primary">
          <div className="metric-value">
            <small>{t("in")}</small> 48 <small>{t("min")}</small>
          </div>
          <div className="metric-foot ellipsis">Halcyon Robotics</div>
        </div>
        <div
          className="meeting-timeline"
          role="img"
          aria-label={t("Next meeting")}
        >
          <div className="time-track" aria-hidden="true">
            <i className="time-elapsed" />
            <i className="time-slot first" />
            <i className="time-slot second" />
            <i className="time-slot third" />
            <b className="time-now" />
          </div>
          <div className="axis">
            <span>8</span>
            <span>10</span>
            <span>12</span>
            <span>2</span>
            <span>4</span>
            <span>6</span>
          </div>
        </div>
      </MetricCard>
    </section>
  );
}

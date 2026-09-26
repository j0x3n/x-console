import React from "react";
import { useT } from "../../contexts/LanguageContext";

export default function SettingsPage() {
  const t = useT();
  return (
    <div className="subpage">
      <div className="eyebrow">xcc</div>
      <h1>{t("Workspace settings")}</h1>
      <section className="plain-section settings-section">
        <h2>{t("Profile")}</h2>
        <div className="work-row">
          <span className="profile-avatar">JO</span>
          <span>
            <b>jo</b>
            <small>{t("Head of Revenue")}</small>
          </span>
        </div>
      </section>
    </div>
  );
}

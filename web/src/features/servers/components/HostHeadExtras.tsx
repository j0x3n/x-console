import { useState } from "react";
import { Pencil } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import { usePageMark } from "../../../stores/page-title";
import type { HostDetail } from "../api";
import CountryFlag from "./CountryFlag";
import { HostInfoDialog } from "./HostInfo";

/**
 * 详情页页头的补充（B82）：左上角名称左边放国旗，页头按钮里加“编辑信息”。
 * 服务器详情和电脑页共用。
 */
export default function HostHeadExtras({ host }: { host: HostDetail }) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  usePageMark(
    host.country ? <CountryFlag country={host.country} /> : null,
    host.country?.code ?? "",
  );
  return (
    <>
      <button
        type="button"
        className="xc-btn"
        title={t("Edit info")}
        onClick={() => setEditing(true)}
      >
        <Pencil size={14} />
        <span className="servers-btn-text">{t("Edit info")}</span>
      </button>
      {editing && (
        <HostInfoDialog host={host} onClose={() => setEditing(false)} />
      )}
    </>
  );
}

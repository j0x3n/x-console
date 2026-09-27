import { Cloud, CloudAlert, CloudUpload } from "lucide-react";
import { useT } from "../../../contexts/LanguageContext";
import type { DriveItem } from "../api";

/** 列表里每个条目的 S3 同步状态。没配置 S3 时不显示。 */
export default function SyncIcon({ item }: { item: DriveItem }) {
  const t = useT();
  if (item.isDir || item.syncState === "off") return null;
  if (item.syncState === "synced")
    return (
      <span className="drive-sync ok" title={t("Synced")}>
        <Cloud size={14} />
      </span>
    );
  if (item.syncState === "pending")
    return (
      <span className="drive-sync" title={t("Waiting to sync")}>
        <CloudUpload size={14} />
      </span>
    );
  return (
    <span
      className="drive-sync danger"
      title={item.syncError || t("Sync failed")}
    >
      <CloudAlert size={14} />
    </span>
  );
}

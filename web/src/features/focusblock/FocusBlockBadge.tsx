import { ShieldAlert, ShieldCheck } from "lucide-react";
import { Link } from "react-router";
import { useT } from "../../contexts/LanguageContext";
import { useFocusBlockStatus } from "./api";
import "./i18n";
import "./focusblock.css";

/**
 * 番茄钟上的“屏蔽中”小标（B131）。有电脑的屏蔽生效时显示“屏蔽中”，
 * 选了电脑但没成功时显示“屏蔽有问题”，点开屏蔽设置看原因。什么都没开时不显示。
 */
export default function FocusBlockBadge() {
  const t = useT();
  const status = useFocusBlockStatus();
  const s = status.data;
  if (!s) return null;
  if (s.problemComputers > 0)
    return (
      <Link
        className="xc-badge warn focusblock-badge"
        to="/calendar/focus?block=1"
        title={t("Block has a problem")}
      >
        <ShieldAlert size={12} /> {t("Block has a problem")}
      </Link>
    );
  if (!s.blocking) return null;
  return (
    <span
      className="xc-badge accent focusblock-badge"
      title={t("Sites and programs are blocked on your computers.")}
    >
      <ShieldCheck size={12} /> {t("Blocking now")}
    </span>
  );
}

import { useLocation } from "react-router";
import { Inbox, MailOpen, Settings2 } from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useMailAccounts } from "./api";

/**
 * 左栏“邮件”的二级菜单：全部收件箱、未读，下面每个邮箱一行（状态点和未读数），
 * 最下面是邮箱设置。邮件页不再放自己的邮箱列和未读开关。
 */
export default function MailNavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const location = useLocation();
  const accounts = useMailAccounts();
  const list = accounts.data ?? [];
  const onMail = location.pathname === "/mail";
  const params = new URLSearchParams(location.search);
  const accountId = Number(params.get("a")) || null;
  const unread = params.get("unread") === "1";
  const total = list.reduce((n, a) => n + a.unread, 0);
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/mail"
          icon={Inbox}
          label={t("All inboxes")}
          active={onMail && !accountId && !unread}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/mail?unread=1"
          icon={MailOpen}
          label={t("Unread")}
          count={total || null}
          active={onMail && !accountId && unread}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      {list.length > 0 && (
        <NavPanelGroup label={t("Mailboxes")}>
          {list.map((a) => (
            <NavPanelLink
              key={a.id}
              to={`/mail?a=${a.id}`}
              mark={<i className={`xc-dot ${accountTone(a.status)}`} />}
              label={a.name}
              count={a.unread || null}
              active={onMail && accountId === a.id}
              onNavigate={onNavigate}
            />
          ))}
        </NavPanelGroup>
      )}
      <NavPanelGroup>
        <NavPanelLink
          to="/settings/mail"
          icon={Settings2}
          label={t("Mailbox settings")}
          active={false}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
    </NavPanelStack>
  );
}

function accountTone(status: string) {
  if (status === "ok") return "ok";
  if (status === "error") return "danger";
  return "warn";
}

import { Suspense } from "react";
import { Outlet } from "react-router";
import PageHeading from "../../components/ui/PageHeading";
import { Loading } from "../../components/ui/States";
import { useLanguage, useT } from "../../contexts/LanguageContext";

/** 日程模块的外框：标题。日历、专注、日历管理的切换在左栏二级菜单里（B103）。早报不在这里，见 BriefsRoute（B66）。 */
export default function CalendarShell() {
  const t = useT();
  const language = useLanguage();
  const today = new Date().toLocaleDateString(
    language === "zh" ? "zh-CN" : "en",
    {
      year: "numeric",
      month: "long",
      day: "numeric",
      weekday: "long",
    },
  );
  return (
    <div className="xc-page calendar-page">
      <PageHeading title={t("Schedule & focus")} subtitle={today} />
      <Suspense fallback={<Loading />}>
        <Outlet />
      </Suspense>
    </div>
  );
}

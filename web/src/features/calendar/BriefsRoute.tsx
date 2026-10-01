import { Suspense, lazy } from "react";
import PageHeading from "../../components/ui/PageHeading";
import { Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";

const BriefsPage = lazy(() => import("./BriefsPage"));

/**
 * 早报历史（B66）。不在日程的标签里了，从今日页右上角进来，
 * 左上角显示“今日 / 早报”（路由的 handle.title 是 My day）。
 * 地址还是 /calendar/briefs，旧链接和通知里的链接照样能打开。
 */
export default function BriefsRoute() {
  const t = useT();
  return (
    <div className="xc-page calendar-page">
      <PageHeading title={t("Daily brief")} />
      <Suspense fallback={<Loading />}>
        <BriefsPage />
      </Suspense>
    </div>
  );
}

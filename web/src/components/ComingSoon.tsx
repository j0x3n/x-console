import { Hammer } from "lucide-react";
import { useT } from "../contexts/LanguageContext";
import PageHeading from "./ui/PageHeading";
import { EmptyState } from "./ui/States";

/** 还没开发的模块先显示这个页面。 */
export default function ComingSoon({ title }: { title: string }) {
  const t = useT();
  return (
    <div className="xc-page">
      <PageHeading title={t(title)} />
      <EmptyState title={t("Coming soon")} icon={<Hammer size={28} />}>
        {t("This module is planned in the roadmap.")}
      </EmptyState>
    </div>
  );
}

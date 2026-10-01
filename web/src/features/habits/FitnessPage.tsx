import { Dumbbell } from "lucide-react";
import { EmptyState } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";

/** 习惯 → 健身：以后放健身动作库。现在先是空页面。 */
export default function FitnessPage() {
  const t = useT();
  return (
    <EmptyState title={t("Exercise library")} icon={<Dumbbell size={28} />}>
      <span>
        {t("Exercises and training plans will live here. It is empty for now.")}
      </span>
    </EmptyState>
  );
}

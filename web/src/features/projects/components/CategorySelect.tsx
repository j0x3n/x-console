import { useT } from "../../../contexts/LanguageContext";
import { UNCATEGORIZED, categoryTree, type Category } from "../logic";

/**
 * 分类下拉，二级分类缩进显示。
 * mode="filter" 时第一项是“所有分类”，还有“未分类”；mode="pick" 时第一项是“未分类”。
 */
export default function CategorySelect({
  categories,
  value,
  onChange,
  mode,
  className = "xc-select",
}: {
  categories: Category[];
  value: number | null;
  onChange: (value: number | null) => void;
  mode: "filter" | "pick";
  className?: string;
}) {
  const t = useT();
  const tree = categoryTree(categories);
  return (
    <select
      className={className}
      value={value ?? ""}
      aria-label={t("Category")}
      onChange={(e) =>
        onChange(e.target.value === "" ? null : Number(e.target.value))
      }
    >
      {mode === "filter" ? (
        <>
          <option value="">{t("All categories")}</option>
          <option value={UNCATEGORIZED}>{t("Uncategorized")}</option>
        </>
      ) : (
        <option value="">{t("Uncategorized")}</option>
      )}
      {tree.map((n) => (
        <option key={n.category.id} value={n.category.id}>
          {n.depth ? `    ${n.category.name}` : n.category.name}
        </option>
      ))}
    </select>
  );
}

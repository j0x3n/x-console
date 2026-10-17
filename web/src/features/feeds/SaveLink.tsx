import { BookmarkPlus } from "lucide-react";
import { errorMessage } from "../../api/client";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useAddLink } from "../readlater/api";

/** “存进稍后读”按钮。用稍后读自己的接口，存原文链接。 */
export default function SaveLink({
  url,
  label,
  compact,
}: {
  url: string;
  /** 读屏软件用，说明存的是哪一条。 */
  label: string;
  compact?: boolean;
}) {
  const t = useT();
  const add = useAddLink();
  const save = () =>
    add.mutate(
      { url },
      {
        onSuccess: (res) =>
          toast(
            res.duplicate
              ? t("This link was saved before")
              : t("Saved to read later"),
          ),
        onError: (err) => toast({ message: errorMessage(err), tone: "error" }),
      },
    );
  if (compact)
    return (
      <button
        className="xc-btn ghost small icon"
        title={t("Save to read later")}
        aria-label={`${t("Save to read later")} ${label}`}
        disabled={add.isPending || !url}
        onClick={save}
      >
        <BookmarkPlus size={15} />
      </button>
    );
  return (
    <button className="xc-btn" disabled={add.isPending || !url} onClick={save}>
      <BookmarkPlus size={14} /> {t("Save to read later")}
    </button>
  );
}

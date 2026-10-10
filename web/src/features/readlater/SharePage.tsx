import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { Bookmark } from "lucide-react";
import { errorMessage } from "../../api/client";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import { useAddLink } from "./api";
import { linkFromShare } from "./format";
import "./i18n";

/**
 * 手机“分享到”X Console 时打开的页面（PWA 的 share_target）。
 * 从分享内容里找出网址，存下来，再回到稍后读。
 */
export default function SharePage() {
  const t = useT();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const add = useAddLink();
  const [error, setError] = useState("");
  const link = linkFromShare(params);
  const sent = useRef(false);

  useEffect(() => {
    if (!link || sent.current) return;
    sent.current = true;
    add.mutate(
      { url: link, source: "share" },
      {
        onSuccess: (res) => {
          toast(
            res.duplicate
              ? t("This link was saved before")
              : t("Saved. Fetching the page…"),
          );
          navigate("/readlater", { replace: true });
        },
        onError: (err) => setError(errorMessage(err)),
      },
    );
    // 只在打开页面时存一次
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [link]);

  let body;
  if (!link)
    body = (
      <EmptyState
        title={t("No web address was found in what you shared.")}
        icon={<Bookmark size={28} />}
      >
        <Link className="xc-btn small" to="/readlater">
          {t("Go to Read later")}
        </Link>
      </EmptyState>
    );
  else if (error)
    body = (
      <EmptyState title={error} icon={<Bookmark size={28} />}>
        <span className="readlater-share-link">{link}</span>
        <Link className="xc-btn small" to="/readlater">
          {t("Go to Read later")}
        </Link>
      </EmptyState>
    );
  else body = <Loading />;

  return (
    <div className="xc-page">
      <PageHeading
        title={t("Read later")}
        subtitle={link && !error ? t("Saving the shared link…") : undefined}
      />
      {body}
    </div>
  );
}

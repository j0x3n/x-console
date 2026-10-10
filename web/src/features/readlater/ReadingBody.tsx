import { useMemo } from "react";
import DOMPurify from "dompurify";

/**
 * 存档的图文。服务端已经清理过一次，这里显示前再清理一次。
 * 图片地址是本站的 /api/v1/readlater/{id}/assets/{hash}，需要登录，同源 Cookie 会带上。
 */
export default function ReadingBody({ html }: { html: string }) {
  const clean = useMemo(
    () =>
      DOMPurify.sanitize(html, {
        USE_PROFILES: { html: true },
        ADD_ATTR: ["target"],
        FORBID_TAGS: ["style", "form", "input", "button", "svg", "math"],
      }),
    [html],
  );
  return (
    <div
      className="readlater-article"
      // eslint-disable-next-line react/no-danger -- 已用 DOMPurify 清理
      dangerouslySetInnerHTML={{ __html: clean }}
    />
  );
}

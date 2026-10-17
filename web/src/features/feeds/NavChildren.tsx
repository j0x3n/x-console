import { useLocation, useSearchParams } from "react-router";
import { MessageSquareText, Newspaper, Tag } from "lucide-react";
import {
  NavPanelGroup,
  NavPanelLink,
  NavPanelStack,
} from "../../components/layout/NavPanel";
import { useT } from "../../contexts/LanguageContext";
import type { NavChildrenProps } from "../../lib/navChildren";
import { useFeedsSettings, useNewsFacets } from "./api";

/** 资讯的二级菜单：新闻、推文，以及最近 30 天最多的标签。 */
export default function NavChildren({ onNavigate }: NavChildrenProps) {
  const t = useT();
  const { pathname } = useLocation();
  const [params] = useSearchParams();
  const tag = params.get("tag") ?? "";
  const onTweets = pathname.startsWith("/feeds/tweets");
  const settings = useFeedsSettings();
  const facets = useNewsFacets(settings.data?.configured === true);
  return (
    <NavPanelStack>
      <NavPanelGroup>
        <NavPanelLink
          to="/feeds"
          icon={Newspaper}
          label={t("News")}
          active={!onTweets && !tag}
          onNavigate={onNavigate}
        />
        <NavPanelLink
          to="/feeds/tweets"
          icon={MessageSquareText}
          label={t("Tweets")}
          active={onTweets}
          onNavigate={onNavigate}
        />
      </NavPanelGroup>
      {!onTweets && !!facets.data?.tags.length && (
        <NavPanelGroup label={t("Tags")}>
          {facets.data.tags.slice(0, 20).map((item) => (
            <NavPanelLink
              key={item.tag}
              to={`/feeds?tag=${encodeURIComponent(item.tag)}`}
              icon={Tag}
              label={item.tag}
              count={item.count}
              active={!onTweets && tag === item.tag}
              onNavigate={onNavigate}
            />
          ))}
        </NavPanelGroup>
      )}
    </NavPanelStack>
  );
}

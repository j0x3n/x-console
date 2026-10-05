import { useMemo, useState } from "react";
import {
  Link,
  Navigate,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router";
import { CircleCheck } from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { SearchBox, Segmented, Toolbar } from "../../components/ui/Toolbar";
import { useT } from "../../contexts/LanguageContext";
import { useMyIssues, useProjects } from "./api";
import IssueList from "./components/IssueList";
import {
  groupIssues,
  isIssueView,
  issuePath,
  issuesInView,
  localDate,
  matchesIssueSearch,
  VIEW_LABELS,
  type IssueGroup,
  type IssueView,
} from "./logic";

type ViewGroup = "project" | "status" | "none";

const EMPTY_TEXT: Record<IssueView, string> = {
  mine: "No open issues",
  today: "No issues due today",
  overdue: "Nothing overdue in any project",
  week: "Nothing due this week",
};

/**
 * 跨项目的卡片视图（B101）：我的未完成、今天到期、已过期、本周到期。
 * 地址 /projects/views/:view，搜索词放在 ?q= 里，左栏的搜索框会带过来。
 */
export default function ViewPage() {
  const { view } = useParams();
  if (!isIssueView(view)) return <Navigate to="/projects" replace />;
  return <IssueViewPage key={view} view={view} />;
}

function IssueViewPage({ view }: { view: IssueView }) {
  const t = useT();
  const navigate = useNavigate();
  const [search, setSearch] = useSearchParams();
  const query = search.get("q") ?? "";
  const [groupBy, setGroupBy] = useState<ViewGroup>("project");
  const issues = useMyIssues();
  const projects = useProjects();

  const inView = useMemo(
    () => issuesInView(view, issues.data ?? [], localDate()),
    [view, issues.data],
  );
  const shown = inView.filter((i) => matchesIssueSearch(i, query));
  const groups = useMemo<IssueGroup[]>(() => {
    if (groupBy === "status") return groupIssues(shown, "status", "due");
    if (groupBy === "none")
      return [{ id: "all", label: "All issues", issues: shown }];
    const names = new Map((projects.data ?? []).map((p) => [p.key, p.name]));
    const order: string[] = [];
    const byProject = new Map<string, typeof shown>();
    for (const issue of shown) {
      if (!byProject.has(issue.projectKey)) {
        byProject.set(issue.projectKey, []);
        order.push(issue.projectKey);
      }
      byProject.get(issue.projectKey)!.push(issue);
    }
    return order.map((key) => ({
      id: key,
      label: names.get(key) ?? key,
      raw: true,
      issues: byProject.get(key)!,
    }));
  }, [groupBy, shown, projects.data]);

  const setQuery = (value: string) => {
    if (value) search.set("q", value);
    else search.delete("q");
    setSearch(search, { replace: true });
  };

  return (
    <div className="xc-page projects-page projects-view-page">
      <PageHeading
        title={t(VIEW_LABELS[view])}
        subtitle={
          issues.data ? (
            <>
              <strong>{inView.length}</strong> {t("open issues")}
            </>
          ) : undefined
        }
      />
      <Toolbar
        end={
          <>
            <SearchBox
              value={query}
              onChange={setQuery}
              placeholder={t("Search issues")}
            />
            <Segmented
              label={t("Group by")}
              value={groupBy}
              onChange={setGroupBy}
              options={[
                { value: "project", label: t("Group by project") },
                { value: "status", label: t("Group by status") },
                { value: "none", label: t("No grouping") },
              ]}
            />
          </>
        }
      />
      {issues.isPending ? (
        <Loading />
      ) : issues.isError ? (
        <ErrorState error={issues.error} onRetry={() => issues.refetch()} />
      ) : shown.length === 0 ? (
        <EmptyState
          title={t(query ? "No matching issues" : EMPTY_TEXT[view])}
          icon={<CircleCheck size={28} />}
        >
          <Link className="xc-btn small" to="/projects">
            {t("See all projects")}
          </Link>
        </EmptyState>
      ) : (
        <IssueList
          groups={groups}
          selectedKey={null}
          onSelect={() => {}}
          onOpen={(key) => navigate(issuePath(key))}
          showProject
        />
      )}
    </div>
  );
}

import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import {
  Archive,
  Columns3,
  Keyboard,
  LayoutList,
  Lock,
  LockOpen,
  Pencil,
  Plus,
  Settings2,
  Star,
  UserRound,
} from "lucide-react";
import PageHeading from "../../components/ui/PageHeading";
import MoreMenu from "../../components/ui/MoreMenu";
import { confirmAction } from "../../components/ui/ConfirmDialog";
import { Segmented, Toolbar } from "../../components/ui/Toolbar";
import { EmptyState, ErrorState, Loading } from "../../components/ui/States";
import { useT } from "../../contexts/LanguageContext";
import { toast } from "../../hooks/useToast";
import {
  useBoardMutations,
  useBoards,
  useCreateIssue,
  useIssues,
  useLabels,
  useMilestones,
  useMoveCard,
  useProjectByKey,
  useUpdateIssue,
  useUpdateProject,
  type Board,
  type BoardList,
} from "./api";
import { useIssueCommands } from "./commands";
import FilterBar from "./components/FilterBar";
import IssueList from "./components/IssueList";
import ListBoard, { confirmDeleteList } from "./components/ListBoard";
import {
  ArchiveDialog,
  BoardDialog,
  ListSettingsDialog,
  MoveCardsDialog,
} from "./components/BoardDialogs";
import NewIssueDialog, { rememberProject } from "./components/NewIssueDialog";
import ProjectDialog from "./components/ProjectDialog";
import ProjectSettingsDialog from "./components/ProjectSettingsDialog";
import ShortcutsDialog from "./components/ShortcutsDialog";
import {
  STATUSES,
  emptyFilter,
  filterIssues,
  groupIssues,
  hasMember,
  issuePath,
  stepSelection,
  type GroupBy,
  type IssueFilter,
  type IssueStatus,
  type SortKey,
} from "./logic";
import { useShortcuts } from "./useShortcuts";

type View = "board" | "list";

interface ViewPrefs {
  view: View;
  groupBy: GroupBy;
  sort: SortKey;
}

const PREFS_KEY = "projects.view";

function loadPrefs(): ViewPrefs {
  const fallback: ViewPrefs = {
    view: "board",
    groupBy: "status",
    sort: "manual",
  };
  try {
    const saved = {
      ...fallback,
      ...JSON.parse(localStorage.getItem(PREFS_KEY) ?? "{}"),
    };
    // B46 去掉了按分类分组。
    if (saved.groupBy === "category") saved.groupBy = "status";
    return saved;
  } catch {
    return fallback;
  }
}

function savePrefs(prefs: ViewPrefs) {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(prefs));
  } catch {
    /* 忽略 */
  }
}

/** 项目页（B46）：上面是看板页签，下面是当前看板的列表和卡片。 */
export default function ProjectPage() {
  const t = useT();
  const navigate = useNavigate();
  const { projectKey } = useParams();
  const { project, isPending, error } = useProjectByKey(projectKey);
  const issues = useIssues(project?.id);
  const boards = useBoards(project?.id);
  const labels = useLabels(project?.id);
  const milestones = useMilestones(project?.id);
  const move = useMoveCard();
  const update = useUpdateIssue();
  const create = useCreateIssue();
  const ops = useBoardMutations(project?.id ?? 0);
  const updateProject = useUpdateProject();
  const [localLock, setLocalLock] = useLocalLock(project?.id);
  const [prefs, setPrefs] = useState(loadPrefs);
  const [filter, setFilter] = useState<IssueFilter>(emptyFilter);
  const [search, setSearch] = useSearchParams();
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [newIssue, setNewIssue] = useState<{
    status: IssueStatus;
    listId?: number;
  } | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [boardDialog, setBoardDialog] = useState<"new" | Board | null>(null);
  const [listSettings, setListSettings] = useState<BoardList | null>(null);
  const [moveAll, setMoveAll] = useState<BoardList | null>(null);
  const [archiveOpen, setArchiveOpen] = useState(false);
  const mine = search.get("mine") === "1";

  useEffect(() => {
    if (project) rememberProject(project.id);
  }, [project]);

  // 当前看板放在地址里（?board=3），刷新和分享都保留。
  const allBoards = boards.data ?? [];
  const boardParam = Number(search.get("board"));
  const board =
    allBoards.find((b) => b.id === boardParam) ?? allBoards[0] ?? undefined;
  const setParam = (key: string, value: string | null) => {
    const params = new URLSearchParams(search);
    if (value === null) params.delete(key);
    else params.set(key, value);
    setSearch(params, { replace: true });
  };

  const changePrefs = (next: Partial<ViewPrefs>) => {
    const merged = { ...prefs, ...next };
    setPrefs(merged);
    savePrefs(merged);
  };

  const onBoard = useMemo(
    () => (issues.data ?? []).filter((i) => i.boardId === board?.id),
    [issues.data, board?.id],
  );
  useIssueCommands(onBoard);
  const visible = useMemo(
    () =>
      filterIssues(onBoard, filter).filter((i) => !mine || hasMember(i, "me")),
    [onBoard, filter, mine],
  );
  const groups = useMemo(
    () => groupIssues(visible, prefs.groupBy, prefs.sort),
    [visible, prefs.groupBy, prefs.sort],
  );
  // J/K 按屏幕上的顺序移动：看板按列表顺序，列表视图按分组顺序。
  const order = useMemo(() => {
    if (prefs.view === "list")
      return groups.flatMap((g) => g.issues.map((i) => i.key));
    const lists = board?.lists ?? [];
    return lists.flatMap((l) =>
      visible
        .filter((i) => i.listId === l.id)
        .sort((a, b) => a.sortOrder - b.sortOrder)
        .map((i) => i.key),
    );
  }, [prefs.view, groups, board, visible]);

  const selectedIssue = issues.data?.find((i) => i.key === selected);
  const open = (key: string) => navigate(issuePath(key));
  const firstList = board?.lists.find((l) => !l.collapsed) ?? board?.lists[0];

  useEffect(() => {
    if (!selected) return;
    document
      .querySelector(`[data-issue-key="${selected}"]`)
      ?.scrollIntoView?.({ block: "nearest" });
  }, [selected]);

  const newCard = () =>
    setNewIssue({
      status: firstList?.status ?? "todo",
      listId: firstList?.id,
    });

  useShortcuts(
    {
      c: newCard,
      n: newCard,
      f: () => {
        const box = document.querySelector<HTMLInputElement>(
          ".projects-toolbar input[type=search], .projects-toolbar input",
        );
        if (!box) return false;
        box.focus();
      },
      q: () => setParam("mine", mine ? null : "1"),
      j: () => setSelected((k) => stepSelection(order, k, 1)),
      k: () => setSelected((k) => stepSelection(order, k, -1)),
      enter: (event) =>
        selected && !(event.target as HTMLElement).closest?.("[data-issue-key]")
          ? open(selected)
          : false,
      e: () =>
        selected ? navigate(`${issuePath(selected)}?edit=title`) : false,
      escape: () => setSelected(null),
      "?": () => setShortcutsOpen(true),
      ...Object.fromEntries(
        STATUSES.map((status, i) => [
          String(i + 1),
          () => {
            if (!selectedIssue) return false;
            if (selectedIssue.status !== status)
              update.mutate({
                issue: selectedIssue,
                key: selectedIssue.key,
                body: { status },
              });
          },
        ]),
      ),
    },
    !!project,
  );

  if (isPending) return <Loading />;
  if (error) return <ErrorState error={error} />;
  if (!project)
    return (
      <div className="xc-page">
        <EmptyState title={t("Project not found")}>
          <Link to="/projects">{t("Back to projects")}</Link>
        </EmptyState>
      </div>
    );
  const archived = !!project.archivedAt;
  // B55：锁定后不能加、改、删看板和列表。后端没上线时记在本机。
  const locked = project.layoutLocked ?? localLock;
  const toggleLock = () => {
    setLocalLock(!locked);
    updateProject.mutate(
      { id: project.id, body: { layoutLocked: !locked } },
      {
        onSuccess: () =>
          toast(locked ? t("Board layout unlocked") : t("Board layout locked")),
      },
    );
  };

  const boardMenu = board && (
    <MoreMenu
      label={`${t("More")}：${board.name}`}
      title={board.name}
      items={[
        ...(locked
          ? []
          : [
              {
                key: "edit",
                label: t("Edit board"),
                onSelect: () => setBoardDialog(board),
              },
              {
                key: "copy",
                label: t("Copy board"),
                onSelect: () =>
                  ops.copyBoard.mutate(board.id, {
                    onSuccess: (b) => setParam("board", String(b.id)),
                  }),
              },
            ]),
        {
          key: "archive-view",
          label: t("Archived items"),
          icon: <Archive size={14} />,
          onSelect: () => setArchiveOpen(true),
        },
        ...(locked
          ? []
          : [
              {
                key: "archive",
                label: t("Archive board"),
                onSelect: async () => {
                  if (
                    await confirmAction({
                      title: `${t("Archive board")}“${board.name}”？`,
                      description: t(
                        "It leaves the tabs. Its cards stay and can be found in search.",
                      ),
                      confirmLabel: t("Archive"),
                      danger: false,
                    })
                  )
                    ops.updateBoard.mutate(
                      { id: board.id, body: { archived: true } },
                      { onSuccess: () => setParam("board", null) },
                    );
                },
              },
              {
                key: "delete",
                label: t("Delete board"),
                danger: true,
                onSelect: async () => {
                  if (
                    await confirmAction({
                      title: `${t("Delete board")}“${board.name}”？`,
                      description: t(
                        "Its cards move to the first other board, into the list for their status.",
                      ),
                      confirmLabel: t("Delete"),
                    })
                  )
                    ops.deleteBoard.mutate(board.id, {
                      onSuccess: () => setParam("board", null),
                    });
                },
              },
            ]),
      ]}
    />
  );

  return (
    <div className="xc-page wide projects-page">
      <PageHeading
        title={project.name}
        subtitle={project.description || undefined}
        aside={
          <>
            <button
              className={`xc-btn small${locked ? " on" : ""}`}
              onClick={toggleLock}
              aria-pressed={locked}
              title={
                locked
                  ? t("Unlock board layout")
                  : t("Lock board layout: no new boards or lists")
              }
              aria-label={
                locked ? t("Unlock board layout") : t("Lock board layout")
              }
            >
              {locked ? <Lock size={14} /> : <LockOpen size={14} />}
            </button>
            <button
              className="xc-btn small"
              onClick={() => setEditOpen(true)}
              title={t("Edit")}
            >
              <Pencil size={14} /> {t("Edit")}
            </button>
            <button
              className="xc-btn small projects-desktop-only"
              onClick={() => setShortcutsOpen(true)}
              aria-label={t("Keyboard shortcuts")}
              title={`${t("Keyboard shortcuts")} (?)`}
            >
              <Keyboard size={14} />
            </button>
            <button
              className="xc-btn small"
              onClick={() => setSettingsOpen(true)}
              aria-label={t("Settings")}
            >
              <Settings2 size={14} />
            </button>
            <button
              className="xc-btn primary small"
              onClick={newCard}
              disabled={archived || !board}
            >
              <Plus size={14} /> {t("New issue")}
              <kbd className="projects-kbd">C</kbd>
            </button>
          </>
        }
      />
      <nav className="projects-board-tabs" aria-label={t("Boards")}>
        {allBoards.map((b) => (
          <button
            key={b.id}
            role="tab"
            aria-selected={b.id === board?.id}
            className={b.id === board?.id ? "on" : ""}
            onClick={() => setParam("board", String(b.id))}
            onDoubleClick={() => !locked && setBoardDialog(b)}
          >
            {b.icon && <span className="projects-board-icon">{b.icon}</span>}
            {b.name}
          </button>
        ))}
        {!archived && !locked && (
          <button
            className="projects-board-add"
            aria-label={t("New board")}
            title={t("New board")}
            onClick={() => setBoardDialog("new")}
          >
            <Plus size={14} />
          </button>
        )}
      </nav>
      <Toolbar
        className="projects-toolbar"
        start={
          <>
            {board && (
              <button
                className={`xc-btn small projects-star${board.starred ? " on" : ""}`}
                aria-pressed={board.starred}
                title={board.starred ? t("Unstar board") : t("Star board")}
                aria-label={board.starred ? t("Unstar board") : t("Star board")}
                onClick={() =>
                  ops.updateBoard.mutate({
                    id: board.id,
                    body: { starred: !board.starred },
                  })
                }
              >
                <Star size={14} />
              </button>
            )}
            <button
              className={`xc-btn small${mine ? " on" : ""}`}
              aria-pressed={mine}
              title={`${t("Only mine")} (Q)`}
              onClick={() => setParam("mine", mine ? null : "1")}
            >
              <UserRound size={14} /> {t("Only mine")}
            </button>
            <FilterBar
              filter={filter}
              onChange={setFilter}
              labels={labels.data ?? []}
              milestones={milestones.data ?? []}
              showStatus={prefs.view === "list"}
              extra={
                prefs.view === "list" && (
                  <>
                    <select
                      className="xc-select projects-filter-select"
                      value={prefs.groupBy}
                      aria-label={t("Group by")}
                      onChange={(e) =>
                        changePrefs({ groupBy: e.target.value as GroupBy })
                      }
                    >
                      <option value="status">{t("Group by status")}</option>
                      <option value="priority">{t("Group by priority")}</option>
                      <option value="none">{t("No grouping")}</option>
                    </select>
                    <select
                      className="xc-select projects-filter-select"
                      value={prefs.sort}
                      aria-label={t("Sort")}
                      onChange={(e) =>
                        changePrefs({ sort: e.target.value as SortKey })
                      }
                    >
                      <option value="manual">{t("Manual order")}</option>
                      <option value="updated">{t("Recently updated")}</option>
                      <option value="priority">{t("Priority")}</option>
                      <option value="due">{t("Due date")}</option>
                    </select>
                  </>
                )
              }
            />
          </>
        }
        end={
          <>
            {boardMenu}
            <Segmented
              label={t("View")}
              value={prefs.view}
              onChange={(view) => changePrefs({ view })}
              options={[
                {
                  value: "board",
                  label: t("Board"),
                  icon: Columns3,
                  iconOnly: true,
                },
                {
                  value: "list",
                  label: t("List"),
                  icon: LayoutList,
                  iconOnly: true,
                },
              ]}
            />
          </>
        }
      />
      {issues.isPending || boards.isPending ? (
        <Loading />
      ) : issues.isError ? (
        <ErrorState error={issues.error} onRetry={() => issues.refetch()} />
      ) : boards.isError ? (
        <ErrorState error={boards.error} onRetry={() => boards.refetch()} />
      ) : !board ? (
        <EmptyState title={t("No boards yet")} />
      ) : prefs.view === "board" ? (
        <ListBoard
          board={board}
          issues={visible}
          selectedKey={selected}
          onSelect={setSelected}
          onOpen={open}
          readOnly={archived}
          locked={locked}
          onMove={(key, plan) =>
            move.mutate({ projectId: project.id, key, plan })
          }
          actions={{
            quickAdd: (listId, title, description) =>
              create.mutateAsync({
                projectId: project.id,
                body: {
                  title,
                  listId,
                  ...(description ? { description } : {}),
                },
              }),
            addList: (name) =>
              ops.createList.mutateAsync({ boardId: board.id, name }),
            settings: setListSettings,
            toggleCollapse: (list) =>
              ops.updateList.mutate({
                id: list.id,
                body: { collapsed: !list.collapsed },
              }),
            moveAll: setMoveAll,
            archiveCards: async (list) => {
              if (
                await confirmAction({
                  title: `${t("Archive all cards")}：${list.name}？`,
                  description: t(
                    "They leave the board and can be restored from Archived items.",
                  ),
                  confirmLabel: t("Archive"),
                  danger: false,
                })
              )
                ops.archiveListCards.mutate(list.id, {
                  onSuccess: (r) =>
                    toast(
                      t("{n} cards archived").replace(
                        "{n}",
                        String(r.archived),
                      ),
                    ),
                });
            },
            archiveList: (list) =>
              ops.updateList.mutate({
                id: list.id,
                body: { archived: true },
              }),
            deleteList: async (list) => {
              if (await confirmDeleteList(list, t))
                ops.deleteList.mutate(list.id);
            },
          }}
        />
      ) : (
        <IssueList
          groups={groups}
          selectedKey={selected}
          onSelect={setSelected}
          onOpen={open}
        />
      )}
      <NewIssueDialog
        open={newIssue !== null}
        onClose={() => setNewIssue(null)}
        projectId={project.id}
        status={newIssue?.status ?? "todo"}
        boardId={board?.id}
        listId={newIssue?.listId}
        onCreated={(issue) => setSelected(issue.key)}
      />
      <BoardDialog
        open={boardDialog !== null}
        onClose={() => setBoardDialog(null)}
        board={boardDialog === "new" ? undefined : (boardDialog ?? undefined)}
        onSubmit={(v) =>
          boardDialog === "new" || boardDialog === null
            ? ops.createBoard.mutateAsync(v).then((b) => {
                setParam("board", String(b.id));
                return b;
              })
            : ops.updateBoard.mutateAsync({
                id: boardDialog.id,
                body: { name: v.name, icon: v.icon },
              })
        }
      />
      <ListSettingsDialog
        list={listSettings}
        onClose={() => setListSettings(null)}
        onSubmit={(id, body) => ops.updateList.mutateAsync({ id, body })}
      />
      <MoveCardsDialog
        list={moveAll}
        boards={allBoards}
        onClose={() => setMoveAll(null)}
        onSubmit={(from, to) => ops.moveListCards.mutateAsync({ from, to })}
      />
      <ArchiveDialog
        board={board}
        open={archiveOpen}
        onClose={() => setArchiveOpen(false)}
        onOpenCard={open}
        onRestoreList={(list) =>
          ops.updateList.mutateAsync({
            id: list.id,
            body: { archived: false },
          })
        }
      />
      <ProjectDialog
        open={editOpen}
        onClose={() => setEditOpen(false)}
        project={project}
      />
      <ShortcutsDialog
        open={shortcutsOpen}
        onClose={() => setShortcutsOpen(false)}
      />
      <ProjectSettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        project={project}
      />
    </div>
  );
}

/** 本机记住的锁定状态，后端还没有 layoutLocked 时用（B55）。 */
function useLocalLock(
  projectId: number | undefined,
): [boolean, (v: boolean) => void] {
  const key = `xc.projects.locked.${projectId ?? 0}`;
  const read = () => {
    try {
      return localStorage.getItem(key) === "1";
    } catch {
      return false;
    }
  };
  const [value, setValue] = useState(read);
  useEffect(() => setValue(read()), [key]); // eslint-disable-line react-hooks/exhaustive-deps
  const set = (v: boolean) => {
    setValue(v);
    try {
      if (v) localStorage.setItem(key, "1");
      else localStorage.removeItem(key);
    } catch {
      // 存不了只在这次有效
    }
  };
  return [value, set];
}

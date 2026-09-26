import AppDialogs from "../components/dialogs/AppDialogs";
import { useWorkspacePreferences } from "../hooks/useWorkspacePreferences";
import { useNavigation } from "../hooks/useNavigation";
import { useToast } from "../hooks/useToast";
import { useWorkspaceState } from "../hooks/useWorkspaceState";
import { useOverlayState } from "../hooks/useOverlayState";
import { useKeyboardShortcuts } from "../hooks/useKeyboardShortcuts";
import React from "react";
import { LanguageContext } from "../contexts/LanguageContext";
import Sidebar from "../components/layout/Sidebar";
import Topbar from "../components/layout/Topbar";
import TodayPage from "../features/today/TodayPage";
import SettingsPage from "../features/settings/SettingsPage";
import WorkspaceRouter from "./WorkspaceRouter";
import Toast from "../components/ui/Toast";

export default function App() {
  const {
    searchOpen,
    setSearchOpen,
    notificationsOpen,
    setNotificationsOpen,
    searchMounted,
    searchClosing,
    notificationsMounted,
    notificationsClosing,
    notificationsRef,
    delegateOpen,
    delegateAssignee,
    openDelegation,
    setDelegateOpen,
    delegateMounted,
    delegateClosing,
    mobileOpen,
    setMobileOpen,
    editItem,
    setEditItem,
    editMounted,
    editClosing,
    drafts,
    setDrafts,
    draftCustomized,
    setDraftCustomized,
    taskText,
    setTaskText,
  } = useOverlayState();
  const { view, navigate } = useNavigation(() => setNotificationsOpen(false));
  const { language, setLanguage, themeMode, setThemeMode, t } =
    useWorkspacePreferences(view);
  const { toast, showToast, hideToast } = useToast();
  const {
    decisions,
    decisionOutcomes,
    decisionHistory,
    exitingDecisions,
    activity,
    tasks,
    setTasks,
    workStatuses,
    setWorkStatuses,
    hireRequest,
    setHireRequest,
    hiredAgents,
    setHiredAgents,
    activeDecision,
    setActiveDecision,
    onAction,
    onUndoDecision,
  } = useWorkspaceState(language, showToast);
  useKeyboardShortcuts({
    view,
    searchOpen,
    delegateOpen,
    editItem,
    decisions,
    activeDecision,
    language,
    setSearchOpen,
    setDelegateOpen,
    setEditItem,
    setNotificationsOpen,
    setMobileOpen,
    setActiveDecision,
    onAction,
  });

  return (
    <LanguageContext.Provider value={language}>
      <div className="app-shell">
        <Sidebar
          view={view}
          decisionCount={decisions.length}
          navigate={navigate}
          openSearch={() => setSearchOpen(true)}
          openDelegate={openDelegation}
          openHire={() => {
            navigate("Crew");
            setHireRequest((value) => value + 1);
          }}
          mobileOpen={mobileOpen}
          setMobileOpen={setMobileOpen}
          language={language}
          setLanguage={setLanguage}
          themeMode={themeMode}
          setThemeMode={setThemeMode}
        />
        <main className="main-panel" id="main">
          <div className="main-scroll">
            <Topbar
              t={t}
              setMobileOpen={setMobileOpen}
              view={view}
              navigate={navigate}
              language={language}
              setHireRequest={setHireRequest}
              openDelegation={openDelegation}
              notificationsRef={notificationsRef}
              notificationsOpen={notificationsOpen}
              setNotificationsOpen={setNotificationsOpen}
              decisions={decisions}
              notificationsMounted={notificationsMounted}
              notificationsClosing={notificationsClosing}
              setSearchOpen={setSearchOpen}
            />
            {view === "Today" ? (
              <TodayPage
                t={t}
                navigate={navigate}
                language={language}
                decisions={decisions}
                exitingDecisions={exitingDecisions}
                activeDecision={activeDecision}
                onAction={onAction}
                setEditItem={setEditItem}
                decisionHistory={decisionHistory}
                onUndoDecision={onUndoDecision}
                activity={activity}
              />
            ) : view === "Settings" ? (
              <SettingsPage />
            ) : (
              <WorkspaceRouter
                view={view}
                language={language}
                t={t}
                navigate={navigate}
                tasks={tasks}
                decisions={decisions}
                decisionHistory={decisionHistory}
                exitingDecisions={exitingDecisions}
                outcomes={decisionOutcomes}
                workStatuses={workStatuses}
                setWorkStatuses={setWorkStatuses}
                hireRequest={hireRequest}
                onHireRequestHandled={() => setHireRequest(0)}
                hiredAgents={hiredAgents}
                setHiredAgents={setHiredAgents}
                onNotify={showToast}
                activity={activity}
                onAction={onAction}
                onUndoDecision={onUndoDecision}
                onEdit={setEditItem}
                openDelegate={openDelegation}
              />
            )}
          </div>
        </main>
        <AppDialogs
          searchMounted={searchMounted}
          setSearchOpen={setSearchOpen}
          navigate={navigate}
          searchClosing={searchClosing}
          delegateMounted={delegateMounted}
          delegateAssignee={delegateAssignee}
          delegateClosing={delegateClosing}
          language={language}
          taskText={taskText}
          setTaskText={setTaskText}
          setDelegateOpen={setDelegateOpen}
          setTasks={setTasks}
          showToast={showToast}
          editMounted={editMounted}
          editClosing={editClosing}
          drafts={drafts}
          draftCustomized={draftCustomized}
          setDrafts={setDrafts}
          setEditItem={setEditItem}
          setDraftCustomized={setDraftCustomized}
          onAction={onAction}
          editItem={editItem}
        />
        {toast && (
          <Toast
            key={toast.id}
            toast={toast}
            hideToast={hideToast}
            onUndoDecision={onUndoDecision}
            language={language}
          />
        )}
      </div>
    </LanguageContext.Provider>
  );
}

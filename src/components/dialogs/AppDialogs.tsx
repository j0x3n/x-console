import type * as Model from "../../types/domain";
import React from "react";
import SearchDialog from "./SearchDialog";
import { DelegateDialog } from "./DelegateDialog";
import { DraftReviewDialog } from "./DraftReviewDialog";
import { companyRecords } from "../../data/workspace";

interface AppDialogsProps {
  searchMounted: boolean;
  setSearchOpen: Model.Setter<boolean>;
  navigate: Model.Navigate;
  searchClosing: boolean;
  delegateMounted: boolean;
  delegateAssignee: string;
  delegateClosing: boolean;
  language: Model.Language;
  taskText: string;
  setTaskText: Model.Setter<string>;
  setDelegateOpen: Model.Setter<boolean>;
  setTasks: Model.Setter<Model.DelegatedTask[]>;
  showToast: Model.Notify;
  editMounted: boolean;
  editClosing: boolean;
  drafts: Record<Model.Language, string>;
  draftCustomized: boolean;
  setDrafts: Model.Setter<Record<Model.Language, string>>;
  setEditItem: Model.Setter<Model.Decision | null>;
  setDraftCustomized: Model.Setter<boolean>;
  onAction: Model.DecisionHandler;
  editItem: Model.Decision | null;
}

export default function AppDialogs({
  searchMounted,
  setSearchOpen,
  navigate,
  searchClosing,
  delegateMounted,
  delegateAssignee,
  delegateClosing,
  language,
  taskText,
  setTaskText,
  setDelegateOpen,
  setTasks,
  showToast,
  editMounted,
  editClosing,
  drafts,
  draftCustomized,
  setDrafts,
  setEditItem,
  setDraftCustomized,
  onAction,
  editItem,
}: AppDialogsProps) {
  return (
    <>
      {searchMounted && (
        <SearchDialog
          close={() => setSearchOpen(false)}
          navigate={navigate}
          closing={searchClosing}
        />
      )}
      {delegateMounted && (
        <DelegateDialog
          initialAssignee={delegateAssignee}
          closing={delegateClosing}
          language={language}
          instruction={taskText}
          setInstruction={setTaskText}
          companies={companyRecords}
          onClose={() => setDelegateOpen(false)}
          onDelegate={(task) => {
            setTasks((prev) => [task, ...prev]);
            setTaskText("");
            setDelegateOpen(false);
            showToast({
              message:
                (language === "zh" ? "已委派给 " : "Delegated to ") +
                task.assignee,
              subtitle: task.company,
              agent: task.assignee,
            });
            navigate("Work");
          }}
        />
      )}
      {editMounted && (
        <DraftReviewDialog
          closing={editClosing}
          language={language}
          draft={drafts[language]}
          showBaseDiff={!draftCustomized}
          setDraft={(value) =>
            setDrafts((prev) => ({ ...prev, [language]: value }))
          }
          onClose={() => setEditItem(null)}
          onSaved={() => {
            setDraftCustomized(true);
            setEditItem(null);
            showToast({
              message: language === "zh" ? "草稿已保存" : "Draft saved",
              subtitle: "Halcyon Robotics",
              agent: "Scribe",
            });
          }}
          onSend={() => {
            if (editItem) onAction(editItem, "approved");
            setEditItem(null);
          }}
        />
      )}
    </>
  );
}

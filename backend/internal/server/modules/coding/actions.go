package coding

import "github.com/j0x3n/x-console/backend/internal/server/actions"

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "coding.launch",
		Title: "新建编码任务",
		Description: "Queue a coding task: an executor (claude or codex, default claude) works on the prompt in a new git worktree " +
			"of a registered repository. With issueKey the issue title and description are put in front of the prompt, and prompt may be empty. " +
			"baseBranch empty means the repository default branch. Returns the task with its id, status and branch.",
		Input: actions.Schema(`{"type":"object","properties":{` +
			`"repoId":{"type":"integer","description":"id of a registered repository"},` +
			`"executor":{"type":"string","enum":["claude","codex"]},` +
			`"prompt":{"type":"string"},` +
			`"baseBranch":{"type":"string"},` +
			`"issueKey":{"type":"string","description":"for example XC-12"}` +
			`},"required":["repoId"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run:    m.actionLaunch,
	})
	m.d.Actions.Register(actions.Action{
		Name:        "coding.list_tasks",
		Title:       "查看编码任务",
		Description: "List coding tasks, running and queued first, then newest first. Optional status filter and limit (default 20).",
		Input: actions.Schema(`{"type":"object","properties":{` +
			`"status":{"type":"array","items":{"type":"string","enum":["queued","running","review","failed","canceled","committed","pushed","pr_opened","discarded"]}},` +
			`"limit":{"type":"integer","minimum":1,"maximum":100}` +
			`},"additionalProperties":false}`),
		Effect: actions.Read,
		Run:    m.actionList,
	})
}

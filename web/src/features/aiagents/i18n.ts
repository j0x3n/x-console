import { registerZh } from "../../lib/i18n";

// B47 Agent 管理。
registerZh({
  // B87 待你决定
  Grant: "批准",
  Answer: "回答",
  Answered: "已回答",
  "Your answer": "你的回答",
  "Agent question": "Agent 提问",
  "Permission request": "权限请求",
  // B87 Agent 通知
  "Agent notifications": "Agent 通知",
  "Where they go follows the rules above.": "发到哪里照上面的规则。",
  "Got a task": "收到任务",
  "A card is assigned to an agent.": "卡片分配给了 Agent。",
  "Started a task": "开始任务",
  "The agent starts working.": "Agent 开始执行。",
  "Needs your decision": "需要你决定",
  "The agent asks for permission or asks you a question, or a coding task waits for review.":
    "Agent 请求权限、问你问题，或者编码任务做完等你审查。",
  "Opened a pull request": "已提 PR",
  "The agent opened a pull request.": "Agent 提了 PR。",
  "Finished a task": "完成任务",
  "The agent is done.": "Agent 做完了。",
  "Failed or stopped": "失败或中断",
  "The task failed, timed out or was stopped.": "任务失败、超时或被中断。",
  // B86 分配给 Agent 开发
  "This board has no repository yet.": "这个看板还没绑定仓库。",
  "Link one and the agent clones it, does the work and opens a pull request.":
    "绑定后 Agent 会同步仓库、开发并提 PR。",
  "Linked to this board": "看板绑定的仓库",
  "Open a pull request when done": "做完自动提 PR",
  // B86 执行日志、删除
  "No agent runs yet": "还没有执行记录",
  "Logs show up here after the agent starts working.":
    "Agent 开始工作后，日志会出现在这里。",
  "Pick a run": "选择一次执行",
  "Open task": "打开任务",
  "No log yet": "还没有日志",
  "Execution log": "执行日志",
  "Agent runs": "Agent 执行",
  "It is working on a task now. Stop the task first.":
    "它正在执行任务，先停止任务再删除。",
  // 能操作的机器（B60）
  "Machines it may operate": "能操作的机器",
  "No servers or computers yet.": "还没有服务器或电脑。",
  "The panel AI can only operate these machines through an agent that has them ticked here.":
    "面板 AI 只能通过这里勾选的 Agent 操作这些机器。",
  Operates: "管",
  Server: "服务器",
  "and N machines": "等 N 台",
  "AI agent": "Agent",
  Tasks: "任务",
  "Git connections": "Git 连接",
  "Git connection": "Git 连接",
  "New agent": "新建 Agent",
  "Edit agent": "编辑 Agent",
  "Create agent": "创建 Agent",
  "Delete agent": "删除 Agent",
  "No agents yet": "还没有 Agent",
  "Agents change code on your machines, or work on cards with the console's tools.":
    "Agent 在你的机器上改代码，或者用面板的工具处理卡片。",
  "For example: a backend developer that only changes the backend folder and runs the tests before it is done.":
    "比如：一个后端开发，只改 backend 目录，做完前先跑测试。",
  "Its finished tasks stay. Cards keep it as a member until you remove it.":
    "它做过的任务会保留。卡片上的成员要你自己去掉。",
  "agents on": "个开着",
  "Waiting for review": "等你看",
  "Cost this month": "本月费用",
  "Over budget": "超出预算",
  "Last task failed": "最近一次任务失败了",
  Disabled: "已停用",
  Default: "默认",
  Avatar: "头像",
  "For example: Backend developer": "比如：后端开发",
  "Claude Code": "Claude Code",
  Codex: "Codex",
  "Built-in": "内置",
  "Runs Claude Code on a machine. Good for changing code.":
    "在机器上跑 Claude Code，适合改代码。",
  "Runs Codex CLI on a machine. Good for changing code.":
    "在机器上跑 Codex CLI，适合改代码。",
  "Uses your AI providers and the console's tools. Good for cards, notes and summaries. Does not touch code.":
    "用你配置的 AI 供应商和面板的工具，适合整理卡片、写笔记、做总结。不碰代码。",
  "The type cannot change after the agent is created.": "建好以后不能改类型。",
  "Pick one of your AI providers' models. It must support tool calls.":
    "从你的 AI 供应商里选一个模型，要支持工具调用。",
  "Passed to the command line as --model. Leave empty for its default.":
    "作为 --model 传给命令行工具。不填用它的默认模型。",
  Instructions: "固定说明",
  "Put at the top of every task. For example: You are the backend developer. Only change the backend folder and run go test before you finish.":
    "每个任务都放在最前面。比如：你是后端开发，只改 backend 目录，做完前跑 go test。",
  "The same rules as API tokens. Running commands, power and settings are never available.":
    "和 API 令牌的规则一样。执行命令、开关机和设置永远不开放。",
  "Default machine": "默认机器",
  "The repository's machine": "仓库所在的机器",
  "Only machines that can run Claude Code or Codex.":
    "只列出能跑 Claude Code 或 Codex 的机器。",
  "Repositories it may change": "允许改的仓库",
  "No repositories yet. Add one on the Repositories tab.":
    "还没有仓库，先到“仓库”页签添加。",
  "Command line permission": "命令行权限",
  "Work folder only": "只能改工作目录",
  "Can only change files in the task's work folder.":
    "只能改这个任务的工作目录里的文件。",
  Full: "不限制",
  "No sandbox and no questions. It can run any command on the machine.":
    "没有沙箱，也不问你。它能在机器上执行任何命令。",
  "Use only on a machine you can afford to break.":
    "只在弄坏了也没关系的机器上用。",
  "Build after each change": "每次改完自动构建",
  "Retries after a failed build": "构建失败后重试次数",
  "Monthly budget (USD)": "每月预算（美元）",
  "When the month's cost reaches the budget, the agent takes no new tasks. Running ones finish.":
    "本月费用到了预算后，不再接新任务，正在跑的会做完。",
  "No tasks yet": "还没有任务",
  "Assign a card to this agent from the card's page.":
    "在卡片页把卡片分配给这个 Agent。",
  "Agents clone, push and open pull requests through these.":
    "Agent 通过这些连接 clone、推送和建 PR。",
  "New connection": "新建连接",
  "No connections yet": "还没有连接",
  "Connect GitHub or your own Forgejo, then add repositories from it.":
    "连上 GitHub 或你自己的 Forgejo，再从里面添加仓库。",
  Webhook: "回调",
  "Add a webhook for pull requests in the repository settings, so merged pull requests move their cards to done. The content type is application/json.":
    "在仓库设置里加一个 PR 事件的回调，PR 合并后卡片会自动到“已完成”。内容类型选 application/json。",
  Secret: "密钥",
  "Check again": "重新检查",
  "Change token": "换令牌",
  "Delete connection": "删除连接",
  "Repositories added from it stay, but can no longer be cloned or open pull requests.":
    "用它添加的仓库会保留，但不能再 clone 和建 PR。",
  "Uses the GitHub module's token": "用 GitHub 模块的令牌",
  Checked: "检查于",
  "Access token": "访问令牌",
  "Saved, but the check failed": "已保存，但检查没通过",
  "Leave empty for github.com. GitHub Enterprise: its API address.":
    "用 github.com 就不填。GitHub 企业版填它的 API 地址。",
  "The address you open Forgejo with.": "你打开 Forgejo 用的地址。",
  "Use the token from the GitHub settings": "用 GitHub 设置里的令牌",
  "Fine-grained token with read and write access to Contents and Pull requests.":
    "细粒度令牌，Contents 和 Pull requests 都要读写权限。",
  "A token with the write:repository scope.":
    "要有 write:repository 权限的令牌。",
  "The token is stored encrypted and never shown again.":
    "令牌加密保存，以后不会再显示。",
  "Assign to an agent": "分配给 Agent",
  "No agent can take work now.": "现在没有能接活的 Agent。",
  "This agent may not change any repository": "这个 Agent 还没有允许改的仓库",
  "The agent's default": "Agent 的默认机器",
  "Extra notes": "补充说明",
  "Optional. The card's title, description, open checklist items and recent comments are sent anyway.":
    "可不填。卡片的标题、描述、没做完的清单和最近的评论都会带上。",
  Assign: "分配",
  "New coding task by hand": "手动建编码任务",
  "The agent started": "Agent 开始干活了",
  "Remove from the card": "从卡片上去掉",
  "Add from a Git connection": "从 Git 连接添加",
  "Find repositories on a machine": "在机器上找仓库",
  "No Git connections yet.": "还没有 Git 连接。",
  "Add one": "去添加",
  "No machine can run tasks": "没有能跑任务的机器",
  "Already added": "已添加",
  "Repository added": "仓库已添加",
  "Cloning…": "正在 clone…",
  "Build steps": "构建步骤",
  "Run in order in the task's work folder after the agent's change. A failed step stops the build. The machine's system picks the set.":
    "Agent 改完后在任务的工作目录里按顺序执行，一步失败就停。按机器的系统选用哪一套。",
  "Step name": "步骤名称",
  Command: "命令",
  Artifacts: "产物",
  "Artifacts, for example dist/** or *.zip, separated by commas":
    "产物，比如 dist/** 或 *.zip，用逗号分开",
  "Timeout in minutes": "超时（分钟）",
  "Add step": "添加一步",
  Build: "构建",
  Building: "构建中",
  "Build passed": "构建通过",
  "Build failed": "构建失败",
  Builds: "构建次数",
  "Build now": "现在构建",
  "The repository has no build steps any more.": "这个仓库已经没有构建步骤了。",
  // 别的模块也有的词，这里再登记一次，保证单独打开这些页面时也是中文。
  Access: "权限",
  Add: "添加",
  Address: "地址",
  "Base branch": "基础分支",
  "Check failed": "检查失败",
  Failed: "失败",
  Idle: "空闲",
  Machine: "机器",
  Model: "模型",
  More: "更多",
  "Move down": "下移",
  "Move up": "上移",
  Name: "名称",
  No: "否",
  "No limit": "不限",
  "No tool calls": "不支持工具调用",
  None: "无",
  "Nothing here": "这里是空的",
  Private: "私有",
  "Read and write": "读写",
  "Read only": "只读",
  "Read, write and delete": "读写和删除",
  Repository: "仓库",
  Saved: "已保存",
  "Search repositories": "搜索仓库",
  System: "系统",
  "Tasks at the same time": "同时运行的任务数",
  Type: "类型",
  Working: "处理中",
  Yes: "是",
  done: "已完成",
  offline: "离线",
  queued: "个排队中",
  repositories: "个仓库",
});

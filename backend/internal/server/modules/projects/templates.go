package projects

import (
	"context"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
)

// B204: project templates. A template adds labels, milestones and starter
// cards with checklists to a new project. See docs/specs/B204.md.

type templateLabel struct{ Name, Color string }

type templateCard struct {
	Title, Description string
	Status             string // empty is backlog
	Milestone          int    // index into milestones, -1 for none
	Labels             []string
	Checklist          string
	Items              []string
}

type projectTemplate struct {
	Description string
	Labels      []templateLabel
	Milestones  []string
	Cards       []templateCard
}

const (
	msConcept = iota
	msPrototype
	msSlice
	msStorePage
	msDemo
	msLaunch
)

var projectTemplates = map[api.CreateProjectTemplate]projectTemplate{
	api.IndieGame: {
		Description: "独立游戏项目，按“先验证再投入”的顺序走六个里程碑：\n\n" +
			"1. **概念验证**：一页设计文档，写清核心玩法循环和范围上限。\n" +
			"2. **可玩原型**：一周内用方块和占位素材做出能玩一轮的灰盒。找 5 个人试玩，再决定做还是砍。\n" +
			"3. **垂直切片**：做出一关正式品质的内容，用它定美术风格、剪预告片。\n" +
			"4. **Steam 商店页上线**：越早越好，开始攒愿望单。\n" +
			"5. **Demo 和新品节**：用可玩的 Demo 参加 Steam 新品节。\n" +
			"6. **正式发售**。\n\n" +
			"每周固定做三件事：发一条开发日志，记一次愿望单数量，找人试玩一次。写代码的卡片可以交给 Agent 任务。",
		Labels: []templateLabel{
			{"玩法", "#2563eb"}, {"程序", "#0d9488"}, {"美术", "#db2777"}, {"音频", "#7c3aed"},
			{"文案", "#d9a227"}, {"发行", "#e5793b"}, {"试玩反馈", "#16a34a"}, {"Bug", "#71717a"},
		},
		// Milestones are listed by name, so the number keeps them in order.
		Milestones: []string{"1 概念验证", "2 可玩原型", "3 垂直切片", "4 Steam 商店页上线", "5 Demo 和新品节", "6 正式发售"},
		Cards: []templateCard{
			{
				Title: "写一页设计文档", Status: "todo", Milestone: msConcept, Labels: []string{"玩法"},
				Description: "只写一页。写不下，说明范围太大。",
				Checklist:   "要写清楚",
				Items: []string{
					"一句话说清这是什么游戏",
					"核心循环：玩家每 30 秒在做什么",
					"找 3 个参考游戏，写清楚和它们哪里不一样",
					"目标玩家是谁，他们平时在哪里看游戏",
					"范围上限：几关、几个角色、最多做几个月",
					"写出这一版不做什么",
				},
			},
			{
				Title: "一周做出灰盒原型", Status: "todo", Milestone: msPrototype, Labels: []string{"玩法", "程序"},
				Description: "只验证核心循环好不好玩。不做美术，不做菜单，不做存档。",
				Checklist:   "完成标准",
				Items: []string{
					"只用方块和占位素材",
					"能从头到尾玩完一轮核心循环",
					"自己连着玩 30 分钟，记下哪里无聊",
					"一周还没做完，先砍范围再继续",
				},
			},
			{
				Title: "找 5 个人试玩原型", Milestone: msPrototype, Labels: []string{"试玩反馈"},
				Description: "不解释规则，只看他们玩。",
				Checklist:   "试玩时",
				Items: []string{
					"准备一个打开就能玩的版本",
					"录屏，或者坐在旁边记笔记",
					"记下卡住、皱眉、笑出来的地方",
					"结束后问：你会推荐给谁",
					"记下有没有人主动问还能不能再玩",
				},
			},
			{
				Title: "决定做还是砍", Milestone: msPrototype, Labels: []string{"玩法"},
				Description: "看试玩结果，不看自己有多喜欢这个点子。砍掉不丢人，回到概念验证换一个点子。",
				Checklist:   "继续做的条件",
				Items: []string{
					"至少一个试玩的人主动想再玩",
					"按现在的速度，范围能在计划时间内做完",
					"想得出商店页上的一句话卖点",
					"三条都不满足就砍掉",
				},
			},
			{
				Title: "做一关正式品质的垂直切片", Milestone: msSlice, Labels: []string{"美术", "音频", "程序"},
				Description: "把一关做到发售时的品质。后面所有内容照这个标准做。",
				Checklist:   "这一关要有",
				Items: []string{
					"定下美术风格和色板",
					"定下音效和音乐风格",
					"新手引导",
					"手柄和键鼠都能玩",
					"录下预告片要用的片段",
				},
			},
			{
				Title: "Steam 商店页上线（即将推出）", Milestone: msStorePage, Labels: []string{"发行"},
				Description: "商店页越早上线越好，愿望单是发售时最重要的数字。Steam 要求发售前商店页至少公开两周。",
				Checklist:   "上线前准备",
				Items: []string{
					"注册 Steamworks，每款游戏付 100 美元的 Steam Direct 费用",
					"各个尺寸的胶囊图",
					"至少 5 张截图，不要只放菜单和过场",
					"预告片 30 到 60 秒，开头几秒就是实际玩法",
					"简短描述和标签",
					"设成“即将推出”",
				},
			},
			{
				Title: "每周发一条开发日志", Milestone: -1, Labels: []string{"发行"},
				Description: "发在 X、Reddit、B 站或者 Steam 社区。一段短视频或者一张动图就够，固定每周同一天发。",
			},
			{
				Title: "每周记一次愿望单数量", Milestone: -1, Labels: []string{"发行"},
				Description: "在 Steamworks 后台看，记在这张卡片的评论里。对照每次发日志、参加活动之后的变化，看哪种宣传有用。",
			},
			{
				Title: "报名 Steam 新品节", Milestone: msDemo, Labels: []string{"发行"},
				Description: "新品节每年办几次，每款游戏只能参加一次，要在 Steamworks 里提前报名。",
				Checklist:   "准备",
				Items: []string{
					"查好下一届的报名截止时间",
					"Demo 单独建一个应用，建议能玩 15 到 30 分钟",
					"Demo 结尾提醒玩家加愿望单",
					"新品节期间准备直播",
					"结束后对比愿望单变化",
				},
			},
			{
				Title: "发售前检查", Milestone: msLaunch, Labels: []string{"发行"},
				Description: "发售日前一个月开始逐条做。",
				Checklist:   "发售前",
				Items: []string{
					"定好价格和各区域价格",
					"成就和云存档",
					"提前两周以上给主播和媒体发 key",
					"准备发售当天的公告和更新说明",
					"发售后一周每天看评论和崩溃报告",
				},
			},
		},
	},
}

// applyProjectTemplate fills a new project from a template.
func (m *Module) applyProjectTemplate(ctx context.Context, projectID int64, name api.CreateProjectTemplate) error {
	tpl, ok := projectTemplates[name]
	if !ok {
		return httpx.Invalid("项目模板不存在")
	}
	labels := map[string]int64{}
	for _, l := range tpl.Labels {
		color := l.Color
		out, err := m.createLabel(ctx, projectID, api.CreateLabel{Name: l.Name, Color: &color})
		if err != nil {
			return err
		}
		labels[l.Name] = out.Id
	}
	milestones := make([]int64, len(tpl.Milestones))
	for i, name := range tpl.Milestones {
		out, err := m.createMilestone(ctx, projectID, api.CreateMilestone{Name: name})
		if err != nil {
			return err
		}
		milestones[i] = out.Id
	}
	for _, c := range tpl.Cards {
		in := issueInput{Title: c.Title, Description: c.Description, Status: c.Status}
		if in.Status == "" {
			in.Status = "backlog"
		}
		if c.Milestone >= 0 {
			in.MilestoneID = &milestones[c.Milestone]
		}
		for _, l := range c.Labels {
			in.LabelIDs = append(in.LabelIDs, labels[l])
		}
		issue, err := m.createIssue(ctx, projectID, in)
		if err != nil {
			return err
		}
		if c.Checklist == "" {
			continue
		}
		var listID int64
		if err := m.d.DB.QueryRowContext(ctx, "INSERT INTO issue_checklists(issue_id,title,position) VALUES(?,?,?) RETURNING id",
			issue.Id, c.Checklist, sortGap).Scan(&listID); err != nil {
			return err
		}
		for i, text := range c.Items {
			if _, err := m.d.DB.ExecContext(ctx, "INSERT INTO issue_checklist_items(checklist_id,text,position) VALUES(?,?,?)",
				listID, text, float64(i+1)*sortGap); err != nil {
				return err
			}
		}
	}
	return nil
}

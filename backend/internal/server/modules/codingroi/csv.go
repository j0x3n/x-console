package codingroi

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/codingroi/api"
)

var outcomeLabels = map[api.CodingROIOutcome]string{
	"merged": "已合并", "reverted": "被撤", "canceled": "已取消", "failed": "失败", "waiting": "待合并", "active": "进行中",
}

var reviewLabels = map[api.CodingROIReviewState]string{
	"done": "", "ongoing": "进行中", "none": "不适用", "no_data": "无记录",
}

func toolLabel(executor api.CodingROITaskExecutor) string {
	if executor == "codex" {
		return "Codex"
	}
	return "Claude Code"
}

// writeCSV writes the task rows for Excel: UTF-8 with a byte order mark, a
// header, and an empty cell where nothing was recorded.
func writeCSV(w io.Writer, rep api.CodingROIReport, loc *time.Location) error {
	if _, err := w.Write([]byte("\xef\xbb\xbf")); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"任务", "标题", "仓库", "工具", "Agent", "结果", "创建时间", "花费美元", "花费不全", "输入 Token", "输出 Token",
		"AI 运行分钟", "审查返工分钟（估算）", "审查时间状态", "审查/运行", "执行次数", "返工轮数", "PR"})
	for _, t := range rep.Tasks {
		_ = cw.Write([]string{
			strconv.FormatInt(t.Id, 10), csvSafe(t.Title), csvSafe(t.RepoName), toolLabel(t.Executor), csvSafe(deref(t.AgentName)),
			outcomeLabels[t.Outcome], t.CreatedAt.In(loc).Format("2006-01-02 15:04:05"),
			num(t.Cost, 4), yes(t.CostPartial), count(t.InputTokens), count(t.OutputTokens),
			minutesOf(t.RunSeconds), num(t.ReviewMinutes, 1), reviewLabels[t.ReviewState], num(t.ReviewPerRun, 2),
			strconv.Itoa(t.Runs), countInt(t.Rounds), csvSafe(deref(t.PrUrl)),
		})
	}
	cw.Flush()
	return cw.Error()
}

func num(v *float64, places int) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', places, 64)
}

func count(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func countInt(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

func yes(b bool) string {
	if b {
		return "是"
	}
	return ""
}

func minutesOf(seconds *int64) string {
	if seconds == nil {
		return ""
	}
	return strconv.FormatFloat(round(float64(*seconds)/60, 1), 'f', 1, 64)
}

// csvSafe stops spreadsheet programs from treating a cell as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@", rune(s[0])) {
		return "'" + s
	}
	return s
}

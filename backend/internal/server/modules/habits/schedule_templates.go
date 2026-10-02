package habits

import "fmt"

type reminderTemplate struct {
	Name, Icon, Unit string
	Target           float64
	Mode             string
	Interval         int
	When, Times      []string
}

func healthTemplate(name string) (reminderTemplate, error) {
	switch name {
	case "", "water":
		return reminderTemplate{Name: "喝水", Icon: "💧", Unit: "杯", Target: 8, Mode: modeInterval, Interval: 60, When: []string{"awake"}}, nil
	case "eyes":
		return reminderTemplate{Name: "护眼", Icon: "👀", Unit: "次", Target: 24, Mode: modeInterval, Interval: 20, When: []string{"active"}}, nil
	case "move":
		return reminderTemplate{Name: "起来活动", Icon: "🚶", Unit: "次", Target: 12, Mode: modeInterval, Interval: 45, When: []string{"active"}}, nil
	case "medicine":
		return reminderTemplate{Name: "吃药", Icon: "💊", Unit: "次", Target: 2, Mode: modeTimes, When: []string{"window"}, Times: []string{"09:00", "21:00"}}, nil
	default:
		return reminderTemplate{}, fmt.Errorf("提醒模板无效")
	}
}

func (f *habitFields) applyTemplate(name string) error {
	if name == "" {
		return nil
	}
	template, err := healthTemplate(name)
	if err != nil {
		return err
	}
	f.Template = name
	f.Name, f.Icon, f.Unit, f.Target = template.Name, template.Icon, template.Unit, template.Target
	f.Mode, f.Interval, f.When, f.Times = template.Mode, template.Interval, template.When, template.Times
	return nil
}

func reminderHint(template string) string {
	switch template {
	case "eyes":
		return "看向 6 米外，休息眼睛 20 秒。"
	case "move":
		return "站起来活动一下。"
	case "water":
		return "喝一杯水。"
	case "medicine":
		return "按医嘱吃药。"
	default:
		return ""
	}
}

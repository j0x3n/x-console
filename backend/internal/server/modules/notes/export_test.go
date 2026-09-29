package notes

import "time"

func (m *Module) SetAIDelayForTest(schedule func(time.Duration, func()) func()) {
	m.aiMu.Lock()
	defer m.aiMu.Unlock()
	m.aiDelay = schedule
}

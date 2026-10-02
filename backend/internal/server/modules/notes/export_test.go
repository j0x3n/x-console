package notes

import "time"

func (m *Module) SetAIDelayForTest(schedule func(time.Duration, func()) func()) {
	m.aiMu.Lock()
	defer m.aiMu.Unlock()
	m.aiDelay = schedule
}

func (m *Module) SetShareClockForTest(now func() time.Time) { m.shareNow = now }

func NoteFingerprintForTest(body string) string          { return noteFingerprint(body) }
func ChangedEnoughForTest(previous, current string) bool { return changedEnough(previous, current) }

var PlainText = plainText

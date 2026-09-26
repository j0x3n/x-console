package focus

import "testing"

func TestIssuePath(t *testing.T) {
	for in, want := range map[string]string{
		"XC-12":   "/projects/XC/12",
		"ABCD-1":  "/projects/ABCD/1",
		"XC":      "/projects",
		"XC-":     "/projects",
		"-3":      "/projects",
		"XC-abc":  "/projects",
		"A-B-100": "/projects/A-B/100",
	} {
		if got := issuePath(in); got != want {
			t.Fatalf("%s: %s", in, got)
		}
	}
}

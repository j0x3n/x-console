//go:build !windows

package coding

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// groupGone reports whether every process of the group led by pid has
// exited (zombies count as gone: in a container nobody may reap them).
func groupGone(pid int) bool {
	deadline := time.Now().Add(3 * time.Second)
	for {
		if !groupAlive(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func groupAlive(pgid int) bool {
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		raw, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		// pid (comm) state ppid pgrp ...
		s := string(raw)
		fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
		if len(fields) > 2 && fields[2] == strconv.Itoa(pgid) && fields[0] != "Z" {
			return true
		}
	}
	return false
}

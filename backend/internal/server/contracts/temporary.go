package contracts

import (
	"path/filepath"
	"sync"
)

var temporaryFiles = struct {
	sync.Mutex
	paths map[string]int
}{paths: map[string]int{}}

func TrackTemporaryFile(path string) func() {
	path = filepath.Clean(path)
	temporaryFiles.Lock()
	temporaryFiles.paths[path]++
	temporaryFiles.Unlock()
	return func() {
		temporaryFiles.Lock()
		defer temporaryFiles.Unlock()
		temporaryFiles.paths[path]--
		if temporaryFiles.paths[path] == 0 {
			delete(temporaryFiles.paths, path)
		}
	}
}

func WithInactiveTemporaryFile(path string, fn func() error) (bool, error) {
	temporaryFiles.Lock()
	defer temporaryFiles.Unlock()
	if temporaryFiles.paths[filepath.Clean(path)] > 0 {
		return false, nil
	}
	return true, fn()
}

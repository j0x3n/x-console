package drive

// SetAfterBlobPutForTest replaces the hook between storing content and
// inserting its row, and returns a function that restores it.
func SetAfterBlobPutForTest(hook func()) func() {
	old := afterBlobPut
	afterBlobPut = hook
	return func() { afterBlobPut = old }
}

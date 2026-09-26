//go:build windows

package coding

import (
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// procGroup controls an executor and everything it starts. On Windows the
// executor is put in a job object that kills all its processes when the job
// is terminated or closed, and in a new console process group so it can get
// Ctrl+Break.
type procGroup struct {
	cmd *exec.Cmd
	job windows.Handle
}

const createNoWindow = 0x08000000

// prepareGroup must be called before cmd.Start.
func prepareGroup(cmd *exec.Cmd) *procGroup {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | createNoWindow,
	}
	return &procGroup{cmd: cmd}
}

// started assigns the process to a kill-on-close job object. Children the
// executor starts afterwards belong to the job too.
func (g *procGroup) started() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(g.cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	g.job = job
	return nil
}

// interrupt sends Ctrl+Break to the executor's process group. This only
// reaches it when the agent shares a console with it; otherwise the kill
// after the grace period does the work.
func (g *procGroup) interrupt() {
	if g.cmd.Process != nil {
		_ = windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(g.cmd.Process.Pid))
	}
}

// kill terminates every process in the job, or the process tree with
// taskkill when the job could not be created.
func (g *procGroup) kill() {
	if g.job != 0 {
		_ = windows.TerminateJobObject(g.job, 1)
		return
	}
	if g.cmd.Process != nil {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(g.cmd.Process.Pid)).Run()
		_ = g.cmd.Process.Kill()
	}
}

// close releases the job handle, which also kills leftovers.
func (g *procGroup) close() {
	if g.job != 0 {
		_ = windows.CloseHandle(g.job)
		g.job = 0
	}
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

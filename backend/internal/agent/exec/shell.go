package exec

// PowerShellCommand is how exec.run starts a command on Windows. Output is
// forced to UTF-8 so the server gets readable text. Kept without a build
// tag so it is tested on Linux.
func PowerShellCommand(command string) (string, []string) {
	return "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command",
		"[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; " + command}
}

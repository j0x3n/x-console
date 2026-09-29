package hostagent

import (
	"encoding/json"
	"testing"
)

func TestClassifyCommand(t *testing.T) {
	tests := []struct {
		command string
		want    Effect
	}{
		{"df -h", Read}, {"  ls -la | grep log | head -n 5  ", Read},
		{"find /var -type f", Read}, {"find /var -delete", Write}, {"find / -exec rm {} +", Write},
		{"top -b -n1", Read}, {"top", Write}, {"ip addr show", Read}, {"ip route add 1.2.3.4", Write},
		{"sed -n 1,3p file", Read}, {"sed -i s/a/b/ file", Write}, {"awk '{print $1}' file", Read}, {"awk 'system(\"id\")' file", Write},
		{"mount", Read}, {"mount /dev/x /mnt", Write}, {"systemctl status nginx", Read}, {"systemctl restart nginx", Write},
		{"docker ps", Read}, {"docker logs x", Read}, {"docker stats --no-stream", Read}, {"docker stats", Write}, {"docker rm x", Write},
		{"Get-Process", Read}, {"dir", Read}, {"ipconfig /all", Read}, {"ipconfig /release", Write},
		{"uptime > /tmp/out", Write}, {"ls < input", Write}, {"ls; rm x", Write}, {"ls && rm x", Write},
		{"ls || rm x", Write}, {"ls `rm x`", Write}, {"ls $(rm x)", Write}, {"ls\nrm x", Write},
		{"ls | rm x", Write}, {"sudo ls", Write}, {"rm -rf /tmp/x", Write},
		{"rm -rf /", Dangerous}, {"rm -r -f /", Dangerous}, {"/bin/rm -rf /etc", Dangerous}, {"sudo  rm  -r  /home", Dangerous},
		{"mkfs.ext4 /dev/sda", Dangerous}, {"dd if=/dev/zero of=/dev/sda", Dangerous},
		{"shutdown -h now", Dangerous}, {"reboot", Dangerous}, {"poweroff", Dangerous}, {"halt", Dangerous},
		{"init 0", Dangerous}, {"init 6", Dangerous}, {":(){ :|:& };:", Dangerous},
		{"iptables -F", Dangerous}, {"nft flush ruleset", Dangerous}, {"ufw disable", Dangerous},
		{"chmod -R 777 /", Dangerous}, {"chown -R user /usr", Dangerous}, {"echo x > /dev/sda", Dangerous},
		{"env rm -rf /tmp/x", Write}, {"date -s tomorrow", Write}, {"sort -o /tmp/x", Write}, {"journalctl --vacuum-time=1d", Write},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			if got := ClassifyCommand(tt.command); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyTool(t *testing.T) {
	for _, tool := range []string{"read_file", "list_dir", "system_info", "list_processes", "list_services", "list_containers"} {
		if got := Classify("host__"+tool, nil); got != Read {
			t.Fatalf("%s: %s", tool, got)
		}
	}
	for _, tool := range []string{"write_file", "service_action", "container_action", "unknown"} {
		if got := Classify("host__"+tool, nil); got != Write {
			t.Fatalf("%s: %s", tool, got)
		}
	}
	if got := Classify("host__run_command", json.RawMessage(`{"command":"df -h"}`)); got != Read {
		t.Fatal(got)
	}
	if got := Classify("host__run_command", json.RawMessage(`{"command":"rm -rf /"}`)); got != Dangerous {
		t.Fatal(got)
	}
}

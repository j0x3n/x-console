package protocol

import "testing"

func TestCountedInterface(t *testing.T) {
	for name, want := range map[string]bool{
		"eth0": true, "ens3": true, "enp0s31f6": true, "wlan0": true, "bond0": true, "br0": true,
		"Ethernet": true, "Wi-Fi": true, "Ethernet 2": true,
		"lo": false, "docker0": false, "veth1a2b3c": false, "br-5f2a1c": false, "virbr0": false,
		"cni0": false, "flannel.1": false, "tun0": false, "tap0": false, "wg0": false,
		"Loopback Pseudo-Interface 1": false, "vEthernet (WSL)": false, "VirtualBox Host-Only Network": false,
		"VMware Network Adapter VMnet8": false, "Hyper-V Virtual Ethernet Adapter": false, "": false,
	} {
		if got := CountedInterface(name); got != want {
			t.Errorf("CountedInterface(%q) = %v, want %v", name, got, want)
		}
	}
}

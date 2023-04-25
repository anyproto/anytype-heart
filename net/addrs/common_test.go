package addrs

import "testing"

func Test_parseInterfaceName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name       string
		args       args
		wantPrefix string
		wantBus    int
		wantNum    int64
	}{
		{"eth0", args{"eth0"}, "eth", 0, 0},
		{"eth1", args{"eth1"}, "eth", 0, 1},
		{"eth10", args{"eth10"}, "eth", 0, 10},
		{"enp0s10", args{"enp0s10"}, "en", 0, 16},
		{"wlp0s20f3", args{"wlp0s20f3"}, "wl", 0, 8435},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrefix, gotBus, gotNum := parseInterfaceName(tt.args.name)
			if gotPrefix != tt.wantPrefix {
				t.Errorf("parseInterfaceName() gotPrefix = %v, want %v", gotPrefix, tt.wantPrefix)
			}
			if gotBus != tt.wantBus {
				t.Errorf("parseInterfaceName() gotBus = %v, want %v", gotBus, tt.wantBus)
			}
			if gotNum != tt.wantNum {
				t.Errorf("parseInterfaceName() gotNum = %v, want %v", gotNum, tt.wantNum)
			}
		})
	}
}

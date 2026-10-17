package wwclient

import (
	"testing"
	"time"
)

func TestParseWWIDFromCmdline(t *testing.T) {
	tests := []struct {
		cmdline     string
		expected    string
		expectError bool
	}{
		{"wwid=123", "123", false},
		{"foo wwid=abc bar", "abc", false},
		{"wwid=", "", true},
		{"foo bar", "", true},
		{"wwid=123 wwid=456", "123", false}, // it takes the first one
	}

	for _, tt := range tests {
		t.Run(tt.cmdline, func(t *testing.T) {
			actual, err := parseWWIDFromCmdline(tt.cmdline)
			if (err != nil) != tt.expectError {
				t.Errorf("parseWWIDFromCmdline(%q) error = %v; expectError %v", tt.cmdline, err, tt.expectError)
				return
			}
			if actual != tt.expected {
				t.Errorf("parseWWIDFromCmdline(%q) = %q; expected %q", tt.cmdline, actual, tt.expected)
			}
		})
	}
}

func TestWWIDFlag(t *testing.T) {
	savedWWID, savedConf := wwid, WarewulfConfArg
	defer func() { wwid, WarewulfConfArg = savedWWID, savedConf }()

	if err := rootCmd.PersistentFlags().Set("wwid", "00:11:22:33:44:55"); err != nil {
		t.Fatal(err)
	}
	if wwid != "00:11:22:33:44:55" {
		t.Errorf("--wwid set wwid = %q", wwid)
	}
	if WarewulfConfArg != savedConf {
		t.Errorf("--wwid changed WarewulfConfArg to %q", WarewulfConfArg)
	}
}

func TestWaitNext(t *testing.T) {
	hup := make(chan struct{}, 1)
	exit := make(chan bool, 1)

	// A SIGHUP that arrived during an update is not lost.
	hup <- struct{}{}
	start := time.Now()
	if waitNext(time.Hour, hup, exit) {
		t.Error("waitNext reported exit for a SIGHUP")
	}
	if time.Since(start) > time.Second {
		t.Error("a pending SIGHUP did not end the wait")
	}

	exit <- true
	if !waitNext(time.Hour, hup, exit) {
		t.Error("waitNext did not report exit")
	}

	start = time.Now()
	if waitNext(10*time.Millisecond, hup, exit) {
		t.Error("waitNext reported exit for the timer")
	}
	if time.Since(start) < 10*time.Millisecond {
		t.Error("waitNext returned before the timer")
	}
}

package installer

import (
	"testing"
)

func TestParseInstallerKeyArrows(t *testing.T) {
	cases := []struct {
		in   []byte
		want string
	}{
		{[]byte{0x1b, '[', 'A'}, "up"},
		{[]byte{0x1b, '[', 'B'}, "down"},
		{[]byte{0x1b, '[', 'C'}, "right"},
		{[]byte{0x1b, '[', 'D'}, "left"},
		{[]byte{0x1b, 'O', 'A'}, "up"},
		{[]byte{0x00, 0x48}, "up"},
		{[]byte{0xe0, 0x50}, "down"},
		{[]byte("\r"), "enter"},
		{[]byte(" "), "space"},
		{[]byte{0x03}, "ctrl-c"},
		{[]byte("a"), "all"},
		{[]byte("Y"), "yes"},
		{[]byte("n"), "no"},
	}
	for _, tc := range cases {
		got := ParseInstallerKeyForTest(tc.in)
		if got != tc.want {
			t.Errorf("parse(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

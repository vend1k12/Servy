package doctor

import "testing"

func TestParseMemInfo(t *testing.T) {
	sample := []byte(`MemTotal:        2048000 kB
MemFree:          123456 kB
MemAvailable:    1024000 kB
Buffers:            1234 kB
`)
	avail, total, ok := parseMemInfo(sample)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if want := uint64(1024000 * 1024); avail != want {
		t.Errorf("avail=%d want %d", avail, want)
	}
	if want := uint64(2048000 * 1024); total != want {
		t.Errorf("total=%d want %d", total, want)
	}
}

func TestParseMemInfoMissingFields(t *testing.T) {
	if _, _, ok := parseMemInfo([]byte("MemFree: 42 kB\n")); ok {
		t.Fatal("expected ok=false when required fields missing")
	}
	if _, _, ok := parseMemInfo(nil); ok {
		t.Fatal("expected ok=false for empty input")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0KiB"},
		{2 * 1024, "2.0KiB"},
		{2 * 1024 * 1024, "2.0MiB"},
		{3 * 1024 * 1024 * 1024, "3.0GiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d)=%q want %q", c.in, got, c.want)
		}
	}
}

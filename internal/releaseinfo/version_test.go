package releaseinfo

import "testing"

func TestVersionOrdering(t *testing.T) {
	for _, tc := range []struct {
		next, current string
		newer         bool
	}{
		{"0.0.1", "0.0.0", true}, {"0.0.1", "0.0.1", false},
		{"0.0.1", "0.0.2", false}, {"0.10.0", "0.9.99", true},
		{"1.0.0", "0.99.99", true}, {"0.0.10", "0.0.9", true},
	} {
		got, err := Newer(tc.next, tc.current)
		if err != nil || got != tc.newer {
			t.Fatalf("%s > %s = %v: %v", tc.next, tc.current, got, err)
		}
	}
}

func TestRejectNonStableOrUnsafeTags(t *testing.T) {
	for _, tag := range []string{"0.0.1", "v0.0", "v00.0.1", "v0.0.1-rc.1", "v0.0.1+dev", "v-1.0.0", "v65536.0.0", "v0.0.1/../x", "v0.0.1\n"} {
		if _, err := TagVersion(tag); err == nil {
			t.Fatalf("accepted %q", tag)
		}
	}
	if version, err := TagVersion("v0.0.1"); err != nil || version != "0.0.1" {
		t.Fatalf("valid tag: %q %v", version, err)
	}
}

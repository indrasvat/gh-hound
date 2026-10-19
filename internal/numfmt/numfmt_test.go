package numfmt

import "testing"

func TestGroupSeparatesThousands(t *testing.T) {
	for n, want := range map[int]string{
		0: "0", 7: "7", 999: "999", 1000: "1,000", 1587: "1,587", 1234567: "1,234,567",
		-123: "-123", -1587: "-1,587",
	} {
		if got := Group(n); got != want {
			t.Errorf("Group(%d) = %q, want %q", n, got, want)
		}
	}
}

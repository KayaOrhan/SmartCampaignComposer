package segment

import "testing"

func TestNormalizeCity(t *testing.T) {
	for _, value := range []string{"Istanbul", "ISTANBUL", "İstanbul", "istanbul", "  İstanbul  ", "I" + string(rune(0x307)) + "stanbul"} {
		if got := NormalizeCity(value); got != "istanbul" {
			t.Errorf("Istanbul variant did not normalize: %q", got)
		}
	}
	if NormalizeCity("Ankara") != "ankara" {
		t.Fatal("ordinary case normalization")
	}
}

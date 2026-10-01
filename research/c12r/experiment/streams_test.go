package experiment

import "testing"

// Go's legacy math/rand source identifies seeds modulo 2^31-1. Check addresses
// for the complete final main forecast and the day-2 validation schedule.
func TestFinalStreamsDoNotOverlap(t *testing.T) {
	const modulus int64 = 2147483647
	seen := make(map[int64]bool)
	add := func(seed int64) {
		k := seed % modulus
		if seen[k] {
			t.Fatalf("duplicate stream %d", k)
		}
		seen[k] = true
	}
	for master := int64(300001); master <= 300032; master++ {
		for profile := int64(0); profile < 5; profile++ {
			base := master*1000000 + profile*150000
			add(base + 149999)
			for day := int64(0); day < 60; day++ {
				add(base + 140000 + day)
				for s := int64(0); s < 64; s++ {
					add(base + 1024*day + s)
				}
				if day%3 == 2 && day < 59 {
					for s := int64(0); s < 1024; s++ {
						add(base + 70000 + 1024*day + s)
					}
				}
			}
		}
	}
}

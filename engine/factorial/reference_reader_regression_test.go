package factorial

import (
	"path/filepath"
	"testing"
)

func TestReferenceReaderResetsEmptyJSONMaps(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "ep")
	if _, e := Run(jobs(t)[4].Config, dir); e != nil {
		t.Fatal(e)
	}
	// This checkpoint follows a ready record and an empty hard_checks map.
	// It used to fail the prefix hash check although the stored chain was valid.
	out := filepath.Join(tmp, "reference")
	if e := ReferenceFromEpisode(dir, 25, out, 9876, 32); e != nil {
		t.Fatal(e)
	}
}

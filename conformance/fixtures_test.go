package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedFixtureIntegrity(t *testing.T) {
	raw, err := os.ReadFile("fixtures.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct{ Files map[string]string }
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	if len(lock.Files) == 0 {
		t.Fatal("empty fixture lock")
	}
	count := 0
	err = filepath.WalkDir("testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		count++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		name := filepath.ToSlash(path)
		if lock.Files[name] != hex.EncodeToString(sum[:]) {
			t.Errorf("fixture %s differs from pinned snapshot", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != len(lock.Files) {
		t.Fatalf("fixture count %d differs from lock count %d", count, len(lock.Files))
	}
}

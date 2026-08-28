package queries

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSQLConstructionGuard is intentionally simple enough to run everywhere,
// including release builds. It prevents the two dangerous construction styles
// that previously appeared in this package: formatted SQL and HTTP-derived
// values entering query text. Optional WHERE/ORDER fragments in this package
// must remain server-owned constants; all values use driver placeholders.
func TestSQLConstructionGuard(t *testing.T) {
	formattedSQL := regexp.MustCompile(`(?is)fmt\.Sprintf\s*\([^)]*\b(SELECT|INSERT|UPDATE|DELETE|LIMIT|OFFSET)\b`)
	httpInput := regexp.MustCompile(`(?i)(c|ctx)\.(Query|Params|Body)|url\.Values|FormValue`)
	err := filepath.Walk(".", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if formattedSQL.Match(contents) {
			t.Errorf("%s formats SQL text; bind values and map fragments to constants", path)
		}
		if httpInput.Match(contents) {
			t.Errorf("%s imports HTTP-derived input into the query layer", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

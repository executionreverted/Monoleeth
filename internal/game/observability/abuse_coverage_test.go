package observability

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPhase12AbuseTestCoverageCoversRequiredCases(t *testing.T) {
	coverage := Phase12AbuseTestCoverage()
	required := RequiredAbuseTestCases()
	if len(coverage) != len(required) {
		t.Fatalf("coverage items = %d, want %d", len(coverage), len(required))
	}

	seen := map[AbuseTestCase]bool{}
	for index, item := range coverage {
		if item.Case != required[index] {
			t.Fatalf("coverage case[%d] = %q, want %q", index, item.Case, required[index])
		}
		if seen[item.Case] {
			t.Fatalf("duplicate abuse coverage case %q", item.Case)
		}
		seen[item.Case] = true
		if len(item.Evidence) == 0 {
			t.Fatalf("abuse coverage %q has no evidence", item.Case)
		}
		for _, evidence := range item.Evidence {
			if evidence.Package == "" || evidence.TestName == "" || evidence.Note == "" {
				t.Fatalf("abuse coverage %q has incomplete evidence: %+v", item.Case, evidence)
			}
		}
	}

	report := NewAbuseTestCoverageReport(coverage)
	if !report.Passed {
		t.Fatalf("phase 12 abuse coverage report failed: missing %#v", report.Missing)
	}
}

func TestAbuseTestCoverageReportFailsClosed(t *testing.T) {
	report := NewAbuseTestCoverageReport(nil)
	if report.Passed {
		t.Fatal("nil abuse coverage report passed")
	}
	assertAbuseTestCases(t, report.Missing, RequiredAbuseTestCases())

	partial := NewAbuseTestCoverageReport([]AbuseTestCoverage{
		{
			Case: AbuseTestNegativeAmounts,
			Evidence: []AbuseTestEvidence{
				{Package: "gameproject/internal/game/foundation", TestName: "TestValidatePositiveAmountRejectsZeroAndNegativeValues", Note: "negative amount coverage"},
			},
		},
		{
			Case:     AbuseTestEnormousAmounts,
			Evidence: nil,
		},
	})
	if partial.Passed {
		t.Fatal("partial abuse coverage report passed")
	}
	assertAbuseTestCases(t, partial.Missing, RequiredAbuseTestCases()[1:])
}

func TestPhase12AbuseEvidenceReferencesExistingGoTests(t *testing.T) {
	repoRoot := repositoryRoot(t)
	for _, item := range Phase12AbuseTestCoverage() {
		for _, evidence := range item.Evidence {
			testExists, err := goTestFunctionExists(repoRoot, evidence.Package, evidence.TestName)
			if err != nil {
				t.Fatalf("abuse coverage %q evidence %+v lookup error: %v", item.Case, evidence, err)
			}
			if !testExists {
				t.Fatalf("abuse coverage %q references missing test %s.%s", item.Case, evidence.Package, evidence.TestName)
			}
		}
	}
}

func TestAbuseTestCoverageSlicesAreCloneSafe(t *testing.T) {
	required := RequiredAbuseTestCases()
	required[0] = AbuseTestCase("mutated")
	if RequiredAbuseTestCases()[0] != AbuseTestNegativeAmounts {
		t.Fatal("required abuse test cases mutated through returned slice")
	}

	coverage := Phase12AbuseTestCoverage()
	coverage[0].Case = AbuseTestCase("mutated")
	coverage[0].Evidence[0].TestName = "mutated"

	next := Phase12AbuseTestCoverage()
	if next[0].Case != AbuseTestNegativeAmounts {
		t.Fatalf("coverage case mutated through returned slice: got %q", next[0].Case)
	}
	if next[0].Evidence[0].TestName == "mutated" {
		t.Fatal("coverage evidence mutated through returned slice")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func goTestFunctionExists(repoRoot, packagePath, testName string) (bool, error) {
	const modulePath = "gameproject/"
	if !strings.HasPrefix(packagePath, modulePath) {
		return false, nil
	}
	packageDir := filepath.Join(repoRoot, filepath.FromSlash(strings.TrimPrefix(packagePath, modulePath)))
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return false, err
	}

	fileset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		filename := filepath.Join(packageDir, entry.Name())
		parsed, err := parser.ParseFile(fileset, filename, nil, 0)
		if err != nil {
			return false, err
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Name.Name == testName {
				return true, nil
			}
		}
	}
	return false, nil
}

func assertAbuseTestCases(t *testing.T, got, want []AbuseTestCase) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("abuse cases length = %d, want %d: got %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("abuse case[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

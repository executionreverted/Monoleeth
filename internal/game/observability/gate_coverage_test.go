package observability

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestPhase12ReleaseGateCoverageCoversRequiredModulesAndChecks(t *testing.T) {
	coverage := Phase12ReleaseGateCoverage()
	modules := RequiredReleaseGateModules()
	checks := RequiredReleaseGateChecks()
	if len(coverage) != len(modules)*len(checks) {
		t.Fatalf("release coverage items = %d, want %d", len(coverage), len(modules)*len(checks))
	}

	allowedModules := stringSet(modules)
	allowedChecks := releaseGateCheckSet(checks)
	seen := map[string]bool{}
	for _, item := range coverage {
		if !allowedModules[item.Module] {
			t.Fatalf("unexpected release module %q", item.Module)
		}
		if !allowedChecks[item.Check] {
			t.Fatalf("unexpected release check %q", item.Check)
		}
		key := item.Module + "\x00" + string(item.Check)
		if seen[key] {
			t.Fatalf("duplicate release coverage for module %q check %q", item.Module, item.Check)
		}
		seen[key] = true
		assertGateCoverageItem(t, item.Status, item.Evidence, item.Note, item.Module, string(item.Check))
	}

	report := NewReleaseGateCoverageReport(coverage)
	if !report.Covered || !report.Passed {
		t.Fatalf("release gate coverage report failed: %+v", report)
	}
}

func TestPhase12CommandSecurityCoverageCoversRegisteredRealtimeOperations(t *testing.T) {
	operations := RequiredCommandSecurityOperations()
	registeredOperations := realtimeOperationRegistryKeys(t)
	assertStringSet(t, operations, registeredOperations)

	coverage := Phase12CommandSecurityCoverage()
	checks := RequiredCommandSecurityChecks()
	if len(coverage) != len(operations)*len(checks) {
		t.Fatalf("command security coverage items = %d, want %d", len(coverage), len(operations)*len(checks))
	}

	allowedOperations := stringSet(operations)
	allowedChecks := commandSecurityCheckSet(checks)
	seen := map[string]bool{}
	for _, item := range coverage {
		if !allowedOperations[item.Command] {
			t.Fatalf("unexpected command %q", item.Command)
		}
		if !allowedChecks[item.Check] {
			t.Fatalf("unexpected command security check %q", item.Check)
		}
		key := item.Command + "\x00" + string(item.Check)
		if seen[key] {
			t.Fatalf("duplicate command security coverage for command %q check %q", item.Command, item.Check)
		}
		seen[key] = true
		assertGateCoverageItem(t, item.Status, item.Evidence, item.Note, item.Command, string(item.Check))
	}

	report := NewCommandSecurityCoverageReport(coverage)
	if !report.Covered || !report.Passed {
		t.Fatalf("command security coverage report failed: %+v", report)
	}
}

func TestGateCoverageReportsFailClosed(t *testing.T) {
	releaseReport := NewReleaseGateCoverageReport(nil)
	if releaseReport.Covered || releaseReport.Passed {
		t.Fatal("nil release coverage report passed")
	}
	if len(releaseReport.Missing) != len(RequiredReleaseGateModules())*len(RequiredReleaseGateChecks()) {
		t.Fatalf("nil release missing = %d, want all module/check pairs", len(releaseReport.Missing))
	}

	commandReport := NewCommandSecurityCoverageReport(nil)
	if commandReport.Covered || commandReport.Passed {
		t.Fatal("nil command security coverage report passed")
	}
	if len(commandReport.Missing) != len(RequiredCommandSecurityOperations())*len(RequiredCommandSecurityChecks()) {
		t.Fatalf("nil command missing = %d, want all command/check pairs", len(commandReport.Missing))
	}
}

func TestReleaseGateCoverageReportFailsWhenOneRequiredEvidenceItemIsMissing(t *testing.T) {
	target := ReleaseGateCoverageMissing{
		Module: RequiredReleaseGateModules()[0],
		Check:  RequiredReleaseGateChecks()[0],
	}
	coverage := Phase12ReleaseGateCoverage()
	filtered := make([]ReleaseGateCoverage, 0, len(coverage)-1)
	removed := false
	for _, item := range coverage {
		if item.Module == target.Module && item.Check == target.Check {
			removed = true
			continue
		}
		filtered = append(filtered, item)
	}
	if !removed {
		t.Fatalf("test setup did not remove required evidence item %+v", target)
	}

	report := NewReleaseGateCoverageReport(filtered)
	if report.Covered || report.Passed {
		t.Fatal("release coverage report passed with one required item missing")
	}
	if len(report.Missing) != 1 || report.Missing[0] != target {
		t.Fatalf("missing = %+v, want [%+v]", report.Missing, target)
	}
}

func TestPhase12GateEvidenceReferencesExistingFilesAndTests(t *testing.T) {
	repoRoot := repositoryRoot(t)
	for _, evidence := range allPhase12GateEvidence() {
		if evidence.Note == "" {
			t.Fatalf("gate evidence has blank note: %+v", evidence)
		}
		if evidence.Package != "" || evidence.TestName != "" {
			if evidence.Package == "" || evidence.TestName == "" {
				t.Fatalf("gate evidence has incomplete Go test reference: %+v", evidence)
			}
			testExists, err := goTestFunctionExists(repoRoot, evidence.Package, evidence.TestName)
			if err != nil {
				t.Fatalf("gate evidence %+v lookup error: %v", evidence, err)
			}
			if !testExists {
				t.Fatalf("gate evidence references missing test %s.%s", evidence.Package, evidence.TestName)
			}
		}
		if evidence.Document != "" {
			if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(evidence.Document))); err != nil {
				t.Fatalf("gate evidence references missing document %q: %v", evidence.Document, err)
			}
		}
		if evidence.Package == "" && evidence.TestName == "" && evidence.Document == "" && evidence.Command == "" {
			t.Fatalf("gate evidence has no reference: %+v", evidence)
		}
	}
}

func TestGateCoverageSlicesAreCloneSafe(t *testing.T) {
	modules := RequiredReleaseGateModules()
	modules[0] = "mutated"
	if RequiredReleaseGateModules()[0] == "mutated" {
		t.Fatal("release gate modules mutated through returned slice")
	}
	operations := RequiredCommandSecurityOperations()
	operations[0] = "mutated"
	if RequiredCommandSecurityOperations()[0] == "mutated" {
		t.Fatal("command operations mutated through returned slice")
	}

	releaseCoverage := Phase12ReleaseGateCoverage()
	releaseCoverage[0].Module = "mutated"
	releaseCoverage[0].Evidence[0].TestName = "mutated"
	if next := Phase12ReleaseGateCoverage(); next[0].Module == "mutated" || next[0].Evidence[0].TestName == "mutated" {
		t.Fatal("release coverage mutated through returned slice")
	}

	commandCoverage := Phase12CommandSecurityCoverage()
	commandCoverage[0].Command = "mutated"
	commandCoverage[0].Evidence[0].TestName = "mutated"
	if next := Phase12CommandSecurityCoverage(); next[0].Command == "mutated" || next[0].Evidence[0].TestName == "mutated" {
		t.Fatal("command coverage mutated through returned slice")
	}
}

func assertGateCoverageItem(t *testing.T, status GateStatus, evidence []GateEvidence, note, owner, check string) {
	t.Helper()
	switch status {
	case GateStatusSatisfied:
		if len(evidence) == 0 {
			t.Fatalf("%s/%s is satisfied without evidence", owner, check)
		}
	case GateStatusNotApplicable:
		if note == "" {
			t.Fatalf("%s/%s is not applicable without a note", owner, check)
		}
	case GateStatusMissing:
		t.Fatalf("%s/%s is marked missing", owner, check)
	default:
		t.Fatalf("%s/%s has invalid gate status %q", owner, check, status)
	}
}

func allPhase12GateEvidence() []GateEvidence {
	var evidence []GateEvidence
	for _, item := range Phase12ReleaseGateCoverage() {
		evidence = append(evidence, item.Evidence...)
	}
	for _, item := range Phase12CommandSecurityCoverage() {
		evidence = append(evidence, item.Evidence...)
	}
	return evidence
}

func realtimeOperationRegistryKeys(t *testing.T) []string {
	t.Helper()
	repoRoot := repositoryRoot(t)
	filename := filepath.Join(repoRoot, "internal", "game", "realtime", "envelope.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse realtime envelope: %v", err)
	}

	constants := realtimeOperationConstants(parsed)
	operations := make([]string, 0)
	ast.Inspect(parsed, func(node ast.Node) bool {
		valueSpec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for index, name := range valueSpec.Names {
			if name.Name != "registeredOperations" {
				continue
			}
			if index >= len(valueSpec.Values) {
				t.Fatalf("registeredOperations has no map literal value")
			}
			registry, ok := valueSpec.Values[index].(*ast.CompositeLit)
			if !ok {
				t.Fatalf("registeredOperations is %T, want map composite literal", valueSpec.Values[index])
			}
			for _, element := range registry.Elts {
				keyValue, ok := element.(*ast.KeyValueExpr)
				if !ok {
					t.Fatalf("registeredOperations element is %T, want key/value", element)
				}
				operation, ok := realtimeOperationKeyString(keyValue.Key, constants)
				if !ok {
					t.Fatalf("unsupported registeredOperations key %T", keyValue.Key)
				}
				operations = append(operations, operation)
			}
		}
		return true
	})
	if len(operations) == 0 {
		t.Fatal("registeredOperations registry keys not found")
	}
	sort.Strings(operations)
	return operations
}

func realtimeOperationConstants(parsed *ast.File) map[string]string {
	constants := map[string]string{}
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valueSpec.Values) == 0 {
				continue
			}
			if len(valueSpec.Names) == 0 {
				continue
			}
			typeIdent, typedOperation := valueSpec.Type.(*ast.Ident)
			isOperationSpec := typedOperation && typeIdent.Name == "Operation"
			for index, name := range valueSpec.Names {
				if index >= len(valueSpec.Values) {
					continue
				}
				literal, ok := valueSpec.Values[index].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				if !isOperationSpec && !strings.HasPrefix(name.Name, "Operation") {
					continue
				}
				value, ok := quotedStringValue(literal.Value)
				if ok {
					constants[name.Name] = value
				}
			}
		}
	}
	return constants
}

func realtimeOperationKeyString(expr ast.Expr, constants map[string]string) (string, bool) {
	switch key := expr.(type) {
	case *ast.Ident:
		value, ok := constants[key.Name]
		return value, ok
	case *ast.BasicLit:
		if key.Kind != token.STRING {
			return "", false
		}
		return quotedStringValue(key.Value)
	case *ast.CallExpr:
		fun, ok := key.Fun.(*ast.Ident)
		if !ok || fun.Name != "Operation" || len(key.Args) != 1 {
			return "", false
		}
		literal, ok := key.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return "", false
		}
		return quotedStringValue(literal.Value)
	default:
		return "", false
	}
}

func quotedStringValue(value string) (string, bool) {
	unquoted, err := strconv.Unquote(value)
	if err != nil {
		return "", false
	}
	return unquoted, true
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func releaseGateCheckSet(values []ReleaseGateCheck) map[ReleaseGateCheck]bool {
	set := make(map[ReleaseGateCheck]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func commandSecurityCheckSet(values []CommandSecurityCheck) map[CommandSecurityCheck]bool {
	set := make(map[CommandSecurityCheck]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func assertStringSet(t *testing.T, got, want []string) {
	t.Helper()
	gotSet := stringSet(got)
	wantSet := stringSet(want)
	if len(gotSet) != len(wantSet) {
		t.Fatalf("strings = %#v, want %#v", got, want)
	}
	for value := range wantSet {
		if !gotSet[value] {
			t.Fatalf("strings = %#v, missing %q", got, value)
		}
	}
	for value := range gotSet {
		if !wantSet[value] {
			t.Fatalf("strings = %#v, unexpected %q", got, value)
		}
	}
}

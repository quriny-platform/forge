package internal

import (
	"os/exec"
	"strings"
	"testing"
)

const modulePath = "github.com/quriny-platform/forge"

// goListDeps returns the import path of pkg and everything it depends on,
// transitively, excluding test-only imports.
func goListDeps(t *testing.T, pkg string) []string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", "-f", "{{.ImportPath}}", pkg)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

// isStdlib reports whether an import path belongs to the standard library.
// Standard library paths have no dot in their first element; every other
// module path starts with a domain name.
func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

func packageExists(t *testing.T, pkg string) bool {
	t.Helper()
	return exec.CommandContext(t.Context(), "go", "list", pkg).Run() == nil
}

// allowedDep reports whether dep is inside one of the allowed module-relative
// prefixes. The empty prefix means the root package only.
func allowedDep(dep string, allowed []string) bool {
	for _, a := range allowed {
		full := modulePath
		if a != "" {
			full += "/" + a
		}
		if dep == full || (a != "" && strings.HasPrefix(dep, full+"/")) {
			return true
		}
	}
	return false
}

// layers maps each package to the import prefixes it may use besides the
// standard library. Mirrors the layer rules in docs/ROADMAP.md.
var layers = []struct {
	pkg     string   // path relative to the module root; "" is the root package
	allowed []string // module-relative prefixes
}{
	{pkg: "", allowed: nil},
	{pkg: "qb", allowed: nil},
	{pkg: "postgres/pgwire", allowed: nil},
	{pkg: "postgres/pgtype", allowed: nil},
	{pkg: "postgres/pgconn", allowed: []string{"postgres/pgwire", "postgres/pgtype"}},
	{pkg: "postgres", allowed: []string{"postgres/pgconn", "postgres/pgwire", "postgres/pgtype", ""}},
	{pkg: "pool", allowed: []string{""}},
	{pkg: "scan", allowed: []string{""}},
}

func TestImportDirection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: runs go list")
	}

	for _, l := range layers {
		t.Run(l.pkg, func(t *testing.T) {
			pkg := modulePath
			if l.pkg != "" {
				pkg += "/" + l.pkg
			}
			// Packages from later milestones do not exist yet.
			if !packageExists(t, pkg) {
				t.Skipf("%s not created yet", pkg)
			}

			for _, dep := range goListDeps(t, pkg) {
				if isStdlib(dep) || dep == pkg || allowedDep(dep, l.allowed) {
					continue
				}
				t.Errorf("%s imports %q, which the layer rules forbid", pkg, dep)
			}
		})
	}
}

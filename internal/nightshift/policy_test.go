package nightshift

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionSourcesPassPolicy(t *testing.T) {
	root := moduleRoot(t)
	problems := auditDir(root)
	if len(problems) != 0 {
		t.Fatalf("policy violations:\n%s", strings.Join(problems, "\n"))
	}
	files := productionFiles(t, root)
	if len(files) < 6 {
		t.Fatalf("expected every production file, found %d", len(files))
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			t.Fatalf("test file audited as production: %s", file)
		}
	}
}

func TestPolicyAllowsOnlyIsolatedUnixPollHelper(t *testing.T) {
	dir := t.TempDir()
	helperDir := filepath.Join(dir, "internal", "nightshift")
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(helperDir, "input_poll_unix.go")
	source := `package nightshift
import "golang.org/x/sys/unix"
func wait() { unix.Poll(nil, 0) }
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if problems := auditFile(path); len(problems) != 0 {
		t.Fatalf("isolated poll helper rejected: %v", problems)
	}
	if err := os.Rename(path, filepath.Join(helperDir, "runtime_unix.go")); err != nil {
		t.Fatal(err)
	}
	if problems := auditFile(filepath.Join(helperDir, "runtime_unix.go")); len(problems) == 0 {
		t.Fatal("unix import was allowed outside the isolated poll helper")
	}
	source = `package nightshift
import "golang.org/x/sys/unix"
func wait() { unix.IoctlGetInt(0, 0) }
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if problems := auditFile(path); len(problems) == 0 {
		t.Fatal("isolated helper was allowed to call a non-poll unix function")
	}
}

func TestPolicyRejectsProhibitedSample(t *testing.T) {
	dir := t.TempDir()
	sample := `package sample
import (
  "net/http"
  "os"
  "os/exec"
)
func main() {
  exec.Command("uname")
  os.ReadFile("secret")
  http.Get("https://example.invalid")
  _ = os.Getenv(os.Args[1])
}
`
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	problems := auditDir(dir)
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"os/exec", "net/http", "ReadFile", "Getenv", "https://"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in:\n%s", want, joined)
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func productionFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func auditDir(root string) []string {
	var problems []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		problems = append(problems, auditFile(path)...)
		return nil
	})
	return problems
}

func auditFile(path string) []string {
	source, err := os.ReadFile(path)
	if err != nil {
		return []string{path + ": " + err.Error()}
	}
	text := string(source)
	var problems []string
	for _, banned := range []string{"CVE-", "http://", "https://", "clipboard", "osc52", "WithMouseCellMotion", "WithMouseAllMotion"} {
		if strings.Contains(text, banned) {
			problems = append(problems, path+": prohibited text "+banned)
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, source, parser.SkipObjectResolution)
	if err != nil {
		return append(problems, path+": "+err.Error())
	}
	for _, imported := range file.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		if forbiddenImport(importPath) && !allowedPlatformImport(importPath, path) {
			problems = append(problems, path+": prohibited import "+importPath)
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			name := fun.Sel.Name
			if ident, ok := fun.X.(*ast.Ident); ok && forbiddenSelector(ident.Name, name) {
				problems = append(problems, path+": prohibited call "+ident.Name+"."+name)
			}
			if ident, ok := fun.X.(*ast.Ident); ok && ident.Name == "unix" && name != "Poll" {
				problems = append(problems, path+": prohibited unix call "+name)
			}
			if ident, ok := fun.X.(*ast.Ident); ok && ident.Name == "os" && name == "Getenv" {
				if !allowedEnvArg(call) {
					problems = append(problems, path+": prohibited call os.Getenv")
				}
			}
		case *ast.Ident:
			if fun.Name == "env" && !allowedEnvArg(call) {
				problems = append(problems, path+": prohibited call env")
			}
		}
		return true
	})
	return problems
}

func allowedPlatformImport(importPath, path string) bool {
	return importPath == "golang.org/x/sys/unix" && strings.HasSuffix(filepath.ToSlash(path), "/internal/nightshift/input_poll_unix.go")
}

func forbiddenImport(path string) bool {
	switch {
	case path == "os/exec" || strings.HasPrefix(path, "os/exec/"):
		return true
	case path == "net" || strings.HasPrefix(path, "net/"):
		return true
	case path == "syscall" || strings.HasPrefix(path, "syscall/"):
		return true
	case path == "unsafe" || path == "plugin" || path == "os/user" || path == "io/ioutil" || path == "log":
		return true
	case strings.HasPrefix(path, "golang.org/x/net") || strings.HasPrefix(path, "golang.org/x/sys") || strings.HasPrefix(path, "golang.org/x/crypto"):
		return true
	case strings.Contains(path, "clipboard") || strings.Contains(path, "osc52"):
		return true
	default:
		return false
	}
}

func forbiddenSelector(pkg, name string) bool {
	switch pkg {
	case "os":
		switch name {
		case "Open", "OpenFile", "Create", "ReadFile", "WriteFile", "ReadDir", "Mkdir", "MkdirAll", "Remove", "RemoveAll", "Rename", "Stat", "Lstat", "Getwd", "Chdir", "Executable", "StartProcess", "FindProcess", "UserHomeDir", "UserConfigDir", "UserCacheDir", "Hostname", "Readlink", "Symlink", "Pipe", "Environ", "LookupEnv", "Setenv", "Clearenv", "DirFS":
			return true
		}
	case "exec":
		return name == "Command" || name == "CommandContext" || name == "LookPath"
	case "http":
		return name == "Get" || name == "Post" || name == "ListenAndServe"
	case "net":
		return name == "Dial" || name == "DialTimeout" || name == "LookupHost" || name == "LookupIP" || name == "LookupAddr"
	case "term":
		return name == "ReadPassword"
	}
	return false
}

func allowedEnvArg(call *ast.CallExpr) bool {
	if len(call.Args) != 1 {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	switch strings.Trim(lit.Value, `"`) {
	case "NO_COLOR", "TERM":
		return true
	default:
		return false
	}
}

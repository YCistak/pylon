package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Text Pylon prints must come from the catalogs, never from a Go literal. The
// GUI has scripts/check-i18n.mjs for its half; this is the daemon's, and it
// exists for the same reason: the language became switchable, and every string
// that was written straight into the code stayed in whichever language its
// author happened to be thinking in. A German user reading "Tarayıcıda Spotify
// izni açılıyor" is not a subtle bug, but nothing failed until someone looked.
//
// Only the calls that reach a human are checked — printing, error values, HTTP
// responses, logs. The repository is full of Turkish that belongs where it is:
// the key-less router's phrase tables, the style profiler's filler words, the
// Turkish examples in the action descriptions handed to the LLM, and CLI
// argument aliases like `pylon work bugün`. Those are input and prompt data,
// not output, and a blanket ban would only teach people to silence this test.
var userFacingCalls = map[string]bool{
	"errors.New": true, "fmt.Errorf": true,
	"fmt.Print": true, "fmt.Printf": true, "fmt.Println": true,
	"fmt.Fprint": true, "fmt.Fprintf": true, "fmt.Fprintln": true,
	"http.Error": true,
	// Logs are read by whoever runs journalctl, so they are exempt from the
	// catalogs — but they are not exempt from being English.
	"log.Print": true, "log.Printf": true, "log.Println": true, "log.Fatalf": true,
}

// turkish is the set that gives a stray Turkish string away. Restricted to
// letters no English word contains, so a legitimately accented loanword in an
// English sentence does not read as a translation leak.
const turkish = "ıİşŞğĞçÇöÖüÜ"

func TestNoUserFacingTextIsHardCoded(t *testing.T) {
	root := repoRoot(t)

	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "build", "dist", "vendor", "models", "locales":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil // not ours to report; the compiler says it louder
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !userFacingCalls[callName(call.Fun)] {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if strings.ContainsAny(lit.Value, turkish) {
					rel, _ := filepath.Rel(root, path)
					offenders = append(offenders, rel+":"+
						itoa(fset.Position(lit.Pos()).Line)+": "+lit.Value)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}

	for _, o := range offenders {
		t.Errorf("hard-coded text that only Turkish readers can read:\n  %s\n"+
			"  Text a user reads belongs in internal/i18n/locales via i18n.T; "+
			"log lines and developer-facing errors stay in English.", o)
	}
}

// callName renders fmt.Println / errors.New / a bare identifier as a string,
// and anything more complicated as "" so it is skipped.
func callName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		if pkg, ok := f.X.(*ast.Ident); ok {
			return pkg.Name + "." + f.Sel.Name
		}
	case *ast.Ident:
		return f.Name
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// repoRoot walks up from the test's directory to the go.mod at the top, so the
// test does not care where it is run from or how deep the package sits.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test directory")
		}
		dir = parent
	}
}

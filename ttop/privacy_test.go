package ttop

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var forbiddenImports = map[string]bool{
	"net": true, "net/http": true, "net/smtp": true, "database/sql": true,
	"crypto/tls": true, "os/exec": true,
}

var forbiddenFixture = []string{
	`"content"`, "password", "api_key", "secret", "BEGIN ", "sk-",
}

func TestProductionHasNoNetworkOrExecImports(t *testing.T) {
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "testdata" || base == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if forbiddenImports[p] {
				t.Errorf("%s imports %s", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProductionDoesNotWrite(t *testing.T) {
	root := filepath.Join("..")
	needles := []string{"os.Create(", "os.WriteFile(", "os.OpenFile("}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := string(data)
		for _, n := range needles {
			if strings.Contains(body, n) {
				t.Errorf("%s writes via %s", path, n)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFixturesContainNoPromptOrSecrets(t *testing.T) {
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.Contains(path, string(filepath.Separator)+"testdata"+string(filepath.Separator)) {
			return nil
		}
		if strings.HasSuffix(path, ".golden") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		low := strings.ToLower(string(data))
		for _, n := range forbiddenFixture {
			if strings.Contains(low, strings.ToLower(n)) && n != `"content"` {
				t.Errorf("%s contains %q", path, n)
			}
			if n == `"content"` && strings.Contains(low, `"content"`) && !strings.Contains(low, "redacted") {
				t.Errorf("%s has content field without REDACTED", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

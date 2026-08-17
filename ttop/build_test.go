package ttop

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReleaseTargetsBuild(t *testing.T) {
	mod, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	targets := [][2]string{
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
	}
	for _, pair := range targets {
		osName, arch := pair[0], pair[1]
		t.Run(osName+"/"+arch, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "ttop")
			cmd := exec.Command("go", "build", "-o", out, "./cmd/ttop")
			cmd.Dir = mod
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+osName, "GOARCH="+arch)
			body, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s\n%s", err, body)
			}
		})
	}
}

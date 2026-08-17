package telemetry

import (
	"fmt"
	"path/filepath"
	"time"
)

// LocalDate is the machine-local calendar day of t.
func LocalDate(t time.Time) string {
	return t.In(time.Local).Format("2006-01-02")
}

// RecordRef is a safe error location: basename and byte offset, no record body.
func RecordRef(path string, offset int64) string {
	base := filepath.Base(path)
	if base == "." || base == "/" || base == "" {
		base = "unknown"
	}
	if offset <= 0 {
		return base
	}
	return fmt.Sprintf("%s:%d", base, offset)
}

func LocateDetail(kind, path string, offset int64) string {
	return kind + " · " + RecordRef(path, offset)
}

package testutil

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/anchore/stereoscope/internal/log"
)

const (
	// TestFixturesDir is the standard directory name for test fixtures.
	// This follows the Go convention of using "testdata" for test assets.
	TestFixturesDir = "testdata"

	// LegacyTestFixturesDir is the previous directory name used for test fixtures.
	// Files here will be automatically migrated to TestFixturesDir when accessed.
	LegacyTestFixturesDir = "test-fixtures"

	// GoldenFileDirName is the subdirectory within TestFixturesDir for golden files.
	GoldenFileDirName = "snapshot"

	// GoldenFileExt is the file extension for golden files.
	GoldenFileExt = ".golden"
)

var (
	// GoldenFileDirPath is the full relative path to the golden files directory.
	GoldenFileDirPath = filepath.Join(TestFixturesDir, GoldenFileDirName)

	// migrationMu protects concurrent fixture migrations
	migrationMu sync.Mutex

	// migratedPaths tracks paths that have already been migrated to avoid duplicate work
	migratedPaths = make(map[string]bool)
)

// GetFixturePath returns the path to a test fixture, automatically migrating
// from the legacy "test-fixtures" directory to "testdata" if needed.
// The migration happens transparently on first access.
func GetFixturePath(t testing.TB, pathParts ...string) string {
	t.Helper()

	newPath := filepath.Join(append([]string{TestFixturesDir}, pathParts...)...)
	legacyPath := filepath.Join(append([]string{LegacyTestFixturesDir}, pathParts...)...)

	newExists := pathExists(newPath)
	legacyExists := pathExists(legacyPath)

	// warn if fixture exists in both locations - this requires manual cleanup
	if newExists && legacyExists {
		warnDuplicateFixture(newPath, legacyPath)
		return newPath
	}

	// check if the new path exists first
	if newExists {
		return newPath
	}

	// check if the legacy path exists and migrate if so
	if legacyExists {
		migrateFixture(t, legacyPath, newPath)
		return newPath
	}

	// neither exists, return the new path (caller will handle the error)
	return newPath
}

// migrateFixture moves a fixture from the legacy path to the new path.
// It creates necessary parent directories and preserves the directory structure.
func migrateFixture(t testing.TB, legacyPath, newPath string) {
	t.Helper()

	migrationMu.Lock()
	defer migrationMu.Unlock()

	// check if already migrated (double-check after acquiring lock)
	if migratedPaths[legacyPath] {
		return
	}

	// check if new path now exists (could have been created by another goroutine)
	if pathExists(newPath) {
		migratedPaths[legacyPath] = true
		return
	}

	// create the parent directory for the new path
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Logf("warning: could not create parent directory for fixture migration: %v", err)
		return
	}

	// move the fixture to the new location
	if err := os.Rename(legacyPath, newPath); err != nil {
		t.Logf("warning: could not migrate fixture from %q to %q: %v", legacyPath, newPath, err)
		return
	}

	t.Logf("migrated fixture: %q -> %q", legacyPath, newPath)
	migratedPaths[legacyPath] = true

	// try to clean up empty parent directories in the legacy path
	cleanupEmptyDirs(filepath.Dir(legacyPath))
}

// cleanupEmptyDirs removes empty directories up to the legacy fixtures root.
func cleanupEmptyDirs(dir string) {
	for dir != "." && dir != LegacyTestFixturesDir {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}

	// also try to remove the legacy root if empty
	if dir == LegacyTestFixturesDir {
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
}

// pathExists returns true if the given path exists.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DangerText wraps text in ANSI escape codes for reverse red to make it highly visible.
func DangerText(s string) string {
	return "\033[7;31m" + s + "\033[0m"
}

// warnDuplicateFixture logs a warning when a fixture exists in both the new and legacy locations.
// This indicates that manual cleanup is needed.
func warnDuplicateFixture(newPath, legacyPath string) {
	log.Warn(DangerText("!!! DUPLICATE FIXTURE DETECTED !!!"))
	log.Warnf(DangerText("Fixture exists in BOTH locations:"))
	log.Warnf(DangerText("  New path:    %s"), newPath)
	log.Warnf(DangerText("  Legacy path: %s"), legacyPath)
	log.Warn(DangerText("Please manually remove the legacy fixture to resolve this conflict."))
}

// GetTestFixturesDir returns the fixture directory path, automatically migrating
// from the legacy "test-fixtures" directory if needed. This is useful for cases
// like afero.NewBasePathFs where you need the entire fixtures directory as a base.
func GetTestFixturesDir(t testing.TB) string {
	t.Helper()

	newExists := pathExists(TestFixturesDir)
	legacyExists := pathExists(LegacyTestFixturesDir)

	// warn if both directories exist - this requires manual cleanup
	if newExists && legacyExists {
		warnDuplicateFixture(TestFixturesDir, LegacyTestFixturesDir)
		return TestFixturesDir
	}

	// if the new directory exists, use it
	if newExists {
		return TestFixturesDir
	}

	// if only the legacy directory exists, migrate it
	if legacyExists {
		migrateFixture(t, LegacyTestFixturesDir, TestFixturesDir)
		return TestFixturesDir
	}

	// neither exists, return the new path (caller will handle any errors)
	return TestFixturesDir
}

package updater

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"jv/internal/config"

	"github.com/creativeprojects/go-selfupdate"
)

const (
	// GitHubRepo is the repository for jv releases
	GitHubRepo = "CostaBrosky/jv"

	// CheckInterval is minimum time between update checks
	CheckInterval = 24 * time.Hour

	// UpdateTimeout is maximum time for update operations
	UpdateTimeout = 5 * time.Minute
)

// Updater handles checking and applying updates
type Updater struct {
	config         *config.Config
	currentVersion string
	selfUpdater    *selfupdate.Updater
}

// NewUpdater creates a new Updater instance
func NewUpdater(cfg *config.Config, version string) (*Updater, error) {
	// Configure selfupdate with SHA256 checksum validation
	su, err := selfupdate.NewUpdater(selfupdate.Config{
		Validator: &selfupdate.ChecksumValidator{
			UniqueFilename: "SHA256SUMS.txt",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create updater: %w", err)
	}

	return &Updater{
		config:         cfg,
		currentVersion: cleanVersion(version),
		selfUpdater:    su,
	}, nil
}

// ShouldCheckForUpdate determines if an update check should be performed
// based on config settings and last check time
func (u *Updater) ShouldCheckForUpdate() bool {
	if !u.config.UpdateConfig.Enabled || !u.config.UpdateConfig.AutoCheck {
		return false
	}

	// Rate limit: check at most once per CheckInterval
	if time.Since(u.config.UpdateConfig.LastCheck) < CheckInterval {
		return false
	}

	return true
}

// CheckForUpdate queries GitHub for the latest release
// Returns nil if no update available or if user skipped this version
func (u *Updater) CheckForUpdate(ctx context.Context) (*selfupdate.Release, error) {
	latest, found, err := u.selfUpdater.DetectLatest(ctx, selfupdate.ParseSlug(GitHubRepo))
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}

	if !found {
		return nil, fmt.Errorf("no releases found")
	}

	// Update last check time
	u.config.UpdateConfig.LastCheck = time.Now()
	if err := u.config.Save(); err != nil {
		// Non-fatal, just log
		fmt.Printf("Warning: failed to save config: %v\n", err)
	}

	// Check if this is a newer version
	// Use simple string comparison since LessOrEqual handles version comparison
	if latest.LessOrEqual(u.currentVersion) {
		return nil, nil // Already up to date
	}

	// Check if user explicitly skipped this version
	if u.config.UpdateConfig.SkipVersion == latest.Version() {
		return nil, nil
	}

	return latest, nil
}

// PerformUpdate downloads and installs the update
// Creates a backup and rolls back on failure
func (u *Updater) PerformUpdate(ctx context.Context, release *selfupdate.Release) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}

	// Create backup before attempting update
	backup := exe + ".backup"
	if err := copyFile(exe, backup); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Perform the update
	if err := selfupdate.UpdateTo(ctx, release.AssetURL, release.AssetName, exe); err != nil {
		// Attempt rollback
		if rollbackErr := os.Rename(backup, exe); rollbackErr != nil {
			return fmt.Errorf("update failed and rollback failed: update error: %w, rollback error: %v", err, rollbackErr)
		}
		return fmt.Errorf("update failed (rolled back): %w", err)
	}

	// Clean up backup after a delay (async)
	go func() {
		time.Sleep(10 * time.Second)
		os.Remove(backup)
	}()

	return nil
}

// SkipVersion marks a version as skipped by the user
func (u *Updater) SkipVersion(version string) error {
	u.config.UpdateConfig.SkipVersion = version
	return u.config.Save()
}

// copyFile creates a copy of the file for backup purposes
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

// cleanVersion removes 'v' prefix if present for consistent comparison
func cleanVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}

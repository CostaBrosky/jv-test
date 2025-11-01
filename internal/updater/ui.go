package updater

import (
	"fmt"
	"strings"

	"jv/internal/theme"

	"github.com/charmbracelet/huh"
	"github.com/creativeprojects/go-selfupdate"
)

// PromptForUpdate shows an interactive prompt asking user if they want to update
// Returns the user's choice: "update", "skip", or "later"
func (u *Updater) PromptForUpdate(release *selfupdate.Release) (string, error) {
	sizeMB := float64(release.AssetByteSize) / 1024 / 1024

	// Build description with size and truncated changelog
	changelog := truncateChangelog(release.ReleaseNotes, 400)
	description := fmt.Sprintf(
		"Download size: %.1f MB\n\n%s",
		sizeMB,
		changelog,
	)

	var action string
	err := huh.NewSelect[string]().
		Title(theme.Subtitle.Render(fmt.Sprintf("Update available: %s → %s", u.currentVersion, release.Version()))).
		Description(theme.Faint.Render(description)).
		Options(
			huh.NewOption(theme.SuccessStyle.Render("Update now"), "update"),
			huh.NewOption(theme.InfoStyle.Render("Skip this version"), "skip"),
			huh.NewOption(theme.WarningStyle.Render("Remind me later"), "later"),
		).
		Value(&action).
		Run()

	if err != nil {
		return "", err
	}

	// Handle skip action
	if action == "skip" {
		if err := u.SkipVersion(release.Version()); err != nil {
			fmt.Printf("Warning: failed to save skip preference: %v\n", err)
		}
	}

	return action, nil
}

// ShowUpdateNotification displays a subtle notification about available update
func ShowUpdateNotification(currentVersion, latestVersion string) {
	fmt.Printf("\n%s Update available: %s → %s %s\n\n",
		theme.InfoStyle.Render("ℹ"),
		theme.Faint.Render(currentVersion),
		theme.CurrentStyle.Render(latestVersion),
		theme.Faint.Render("(run 'jv update')"))
}

// ShowUpdateSuccess displays success message after update
func ShowUpdateSuccess(version string) {
	fmt.Println()
	title := theme.SuccessStyle.Padding(0, 2).Render("✓ Update Complete!")
	fmt.Println(theme.SuccessBox.Render(title))
	fmt.Println()
	fmt.Printf("%s Updated to version %s\n",
		theme.LabelStyle.Render("Version:"),
		theme.CurrentStyle.Render(version))
	fmt.Println()
	fmt.Println(theme.Faint.Render("Note: Restart your terminal or run jv again to use the new version."))
	fmt.Println()
}

// ShowAlreadyUpToDate displays message when already on latest version
func ShowAlreadyUpToDate(version string) {
	fmt.Println(theme.SuccessMessage(fmt.Sprintf("You're already running the latest version (%s)", version)))
}

// ShowCheckingForUpdates displays a message while checking for updates
func ShowCheckingForUpdates() {
	fmt.Println(theme.InfoStyle.Render("Checking for updates..."))
}

// ShowDownloadingUpdate displays a message while downloading
func ShowDownloadingUpdate(version string) {
	fmt.Println()
	fmt.Println(theme.InfoStyle.Render(fmt.Sprintf("Downloading jv %s...", version)))
}

// ShowVerifyingChecksum displays a message while verifying
func ShowVerifyingChecksum() {
	fmt.Println(theme.InfoStyle.Render("Verifying checksum..."))
}

// ShowInstallingUpdate displays a message while installing
func ShowInstallingUpdate() {
	fmt.Println(theme.InfoStyle.Render("Installing update..."))
}

// truncateChangelog truncates the changelog to a maximum length
func truncateChangelog(changelog string, maxLen int) string {
	// Clean up common markdown/formatting
	changelog = strings.TrimSpace(changelog)

	// If empty, provide default message
	if changelog == "" {
		return "See release notes on GitHub for details."
	}

	// Truncate if too long
	if len(changelog) <= maxLen {
		return changelog
	}

	// Find a good break point (newline or space)
	truncated := changelog[:maxLen]
	if idx := strings.LastIndex(truncated, "\n"); idx > maxLen/2 {
		truncated = truncated[:idx]
	} else if idx := strings.LastIndex(truncated, " "); idx > maxLen/2 {
		truncated = truncated[:idx]
	}

	return truncated + "..."
}

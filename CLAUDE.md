# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`jv` (Java Version Switcher) is a Windows CLI tool for managing Java installations. It provides an interactive TUI for detecting, switching, and installing Java versions, with permanent environment variable configuration through the Windows Registry.

## Commands

### Build
```powershell
# Development build (version shows as "dev")
go build -ldflags="-s -w" -o jv.exe .

# Build with version info (required for auto-update to work)
go build -ldflags="-s -w -X main.Version=v1.0.0" -o jv.exe .
```

**Important:** The version MUST be injected via `-ldflags` for the auto-update feature to work correctly. The GitHub Actions workflow automatically injects the version from git tags.

### Run
```powershell
go run . <command>
```

Examples:
```powershell
go run . list
go run . switch
go run . install
go run . update
```

### Module Management
```powershell
go mod tidy          # Clean up dependencies
go mod download      # Download dependencies
```

## Architecture

### Core Components

1. **main.go** - Command router and handlers
   - All commands are handled as functions: `handleList()`, `handleUse()`, `handleSwitch()`, etc.
   - Uses Charm Bracelet libraries (Huh for prompts, Lip Gloss for styling)
   - Commands that modify system environment require admin privileges

2. **internal/java/** - Java detection and version management
   - `detector.go`: Scans standard paths + custom paths for Java installations
   - `version.go`: Represents a Java version with path and metadata
   - `spinner.go`: Loading UI for long-running scans
   - Detection strategy: checks for `bin/java.exe`, extracts version from `java -version` output or directory name

3. **internal/config/** - Configuration persistence
   - Location: `%USERPROFILE%\.config\jv\jv.json` (XDG standards)
   - Stores: custom Java paths, search paths, installed JDKs with scope info
   - All path operations are case-insensitive and normalized

4. **internal/env/** - Windows Registry manipulation
   - `windows.go`: System environment variable management via Registry
   - Sets `JAVA_HOME` in `HKLM\System\CurrentControlSet\Control\Session Manager\Environment`
   - Manages PATH: removes old Java entries, prepends `%JAVA_HOME%\bin`
   - Broadcasts `WM_SETTINGCHANGE` to notify applications
   - Requires Administrator privileges

5. **internal/installer/** - Interactive Java installation
   - `installer.go`: Main installation workflow (distributor selection, version selection, scope selection)
   - `distributor.go`: Interface for Java distributors
   - `adoptium.go`: Eclipse Adoptium (Temurin) implementation
   - `downloader.go`: HTTP download with progress bar
   - Supports both system-wide (`C:\Program Files\`) and user-level (`%USERPROFILE%\.jv\`) installs

6. **internal/theme/** - Centralized styling
   - `theme.go`: All Lip Gloss styles and color definitions
   - Consistent orange accent color (#FF8C00) for Java version highlighting
   - Provides styled helpers: `SuccessMessage()`, `ErrorMessage()`, `InfoMessage()`, etc.

7. **internal/updater/** - Auto-update system
   - `updater.go`: Core update logic using go-selfupdate library
   - `ui.go`: Update prompts and notifications with Charm
   - Checks GitHub Releases API for latest version
   - SHA256 checksum verification for security
   - Atomic binary replacement with rollback on failure
   - Background update checks (24hr rate limit)

### Key Design Patterns

- **Admin Detection**: `env.IsAdmin()` checks Windows token membership in Administrators group
- **Path Deduplication**: All Java paths are deduplicated using case-insensitive map keys
- **Scope Tracking**: JDKs installed via `jv install` are tagged as "system" or "user" scope
- **Registry as Source of Truth**: `env.GetJavaHome()` reads from Registry, not process environment
- **Interactive Confirmations**: Critical operations (switch, install, remove) require confirmation prompts
- **Spinner UIs**: Long operations (scanning, downloading) show progress spinners

### File Organization

```
jv/
├── main.go                    # Entry point, command routing
├── internal/
│   ├── config/               # Config file management
│   ├── env/                  # Windows Registry operations
│   ├── installer/            # Java installation system
│   │   ├── adoptium.go      # Adoptium API integration
│   │   ├── distributor.go   # Distributor interface
│   │   ├── downloader.go    # HTTP download with progress
│   │   └── installer.go     # Interactive install workflow
│   ├── java/                 # Java detection
│   ├── theme/                # UI styling
│   └── updater/              # Auto-update system
│       ├── updater.go       # Core update logic
│       └── ui.go            # Update UI components
├── install.ps1               # PowerShell installer script
└── uninstall.ps1             # PowerShell uninstaller
```

## Important Constraints

- **Windows-only**: Uses Windows Registry, `golang.org/x/sys/windows`
- **Administrator for env changes**: `jv use`, `jv switch`, `jv install` (system-wide), `jv repair` require admin
- **Case-insensitive paths**: All path comparisons use `strings.EqualFold()` or `strings.ToLower()`
- **XDG config location**: Config stored at `%USERPROFILE%\.config\jv\jv.json`
- **PATH format**: Always uses `%JAVA_HOME%\bin` variable reference in PATH, not hardcoded paths
- **Update checks**: Background checks limited to once per 24 hours to respect GitHub API rate limits

## Theming System

All UI elements must use the centralized theme from `internal/theme/theme.go`:

- **Colors**: `Primary` (orange #FF8C00), `Success` (green), `Error` (red), `Warning` (yellow), `Info` (blue)
- **Styles**: Use `theme.SuccessStyle`, `theme.ErrorStyle`, `theme.CurrentStyle` (orange Java version), etc.
- **Helpers**: `theme.SuccessMessage()`, `theme.ErrorMessage()`, `theme.PathStyle`, `theme.Code`

When adding new UI elements, extend the theme instead of creating ad-hoc styles.

## Development Notes

- **No tests**: This project currently has no test files
- **PowerShell integration**: Installer script creates config, adds to PATH, sets up completion (includes `update` command)
- **Charm Bracelet libraries**: Huh for forms/prompts, Lip Gloss for styling, Bubbles for components
- **Version extraction**: Prefers `java -version` output, falls back to directory name parsing
- **Auto-update library**: Uses `github.com/creativeprojects/go-selfupdate` for safe binary replacement
- **Update security**: SHA256 checksum verification, HTTPS-only downloads, rollback on failure
- **Version format**: Build with `-ldflags="-X main.Version=vX.Y.Z"` for proper update detection

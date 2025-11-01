package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	HWND_BROADCAST   = 0xFFFF
	WM_SETTINGCHANGE = 0x001A
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	sendMessageW     = user32.NewProc("SendMessageW")
	systemEnvRegPath = `System\CurrentControlSet\Control\Session Manager\Environment`
)

// SetJavaHome sets the JAVA_HOME environment variable system-wide
func SetJavaHome(javaPath string) error {
	// Normalize the path
	javaPath = filepath.Clean(javaPath)

	// Open the system environment registry key
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, systemEnvRegPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key (run as administrator): %w", err)
	}
	defer key.Close()

	// Get current PATH
	currentPath, _, err := key.GetStringValue("Path")
	if err != nil {
		return fmt.Errorf("failed to read PATH: %w", err)
	}

	// Get current JAVA_HOME (if exists)
	oldJavaHome, _, _ := key.GetStringValue("JAVA_HOME")

	// Update JAVA_HOME
	if err := key.SetStringValue("JAVA_HOME", javaPath); err != nil {
		return fmt.Errorf("failed to set JAVA_HOME: %w", err)
	}

	// Update PATH - remove old Java paths and add new one
	newPath := updatePath(currentPath, oldJavaHome, javaPath)
	if err := key.SetExpandStringValue("Path", newPath); err != nil {
		return fmt.Errorf("failed to update PATH: %w", err)
	}

	// Broadcast WM_SETTINGCHANGE to notify all windows
	broadcastSettingChange()

	return nil
}

// updatePath updates the PATH variable by removing old Java paths and adding the new one
func updatePath(currentPath, oldJavaHome, newJavaHome string) string {
	// Split PATH into components
	paths := strings.Split(currentPath, ";")
	newPaths := make([]string, 0, len(paths)+1)

	// Remove old Java-related paths
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Skip if this is the old JAVA_HOME\bin or contains the old Java installation
		if oldJavaHome != "" {
			oldJavaBin := filepath.Join(oldJavaHome, "bin")
			if strings.EqualFold(p, oldJavaBin) || strings.EqualFold(p, "%JAVA_HOME%\\bin") {
				continue
			}
			// Also skip if path contains old java home
			if strings.Contains(strings.ToLower(p), strings.ToLower(oldJavaHome)) {
				continue
			}
		}

		// Skip any existing %JAVA_HOME%\bin references
		if strings.Contains(strings.ToUpper(p), "%JAVA_HOME%") {
			continue
		}

		newPaths = append(newPaths, p)
	}

	// Add new Java bin path using %JAVA_HOME% variable
	newPaths = append([]string{"%JAVA_HOME%\\bin"}, newPaths...)

	return strings.Join(newPaths, ";")
}

// broadcastSettingChange sends a WM_SETTINGCHANGE message to notify all windows
func broadcastSettingChange() {
	env := syscall.StringToUTF16Ptr("Environment")
	sendMessageW.Call(
		uintptr(HWND_BROADCAST),
		uintptr(WM_SETTINGCHANGE),
		0,
		uintptr(unsafe.Pointer(env)),
	)
}

// GetJavaHome returns the current JAVA_HOME value from system environment
func GetJavaHome() (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, systemEnvRegPath, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	value, _, err := key.GetStringValue("JAVA_HOME")
	if err != nil {
		return "", fmt.Errorf("JAVA_HOME not set: %w", err)
	}

	return value, nil
}

// IsAdmin checks if the current process is running with administrator privileges
func IsAdmin() bool {
	var sid *windows.SID

	// Get SID for Administrators group
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	// Get current process token
	token := windows.Token(0)

	// Check if current token is member of administrators group
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}

	return member
}

// RefreshCurrentSession updates environment variables in the current PowerShell session
// This allows users to use 'java -version' immediately without reopening the terminal
func RefreshCurrentSession() error {
	// Get the updated JAVA_HOME from registry
	javaHome, err := GetJavaHome()
	if err != nil {
		return fmt.Errorf("failed to get JAVA_HOME: %w", err)
	}

	// Get the updated PATH from registry
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, systemEnvRegPath, registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	systemPath, _, err := key.GetStringValue("Path")
	if err != nil {
		return fmt.Errorf("failed to read PATH: %w", err)
	}

	// Expand %JAVA_HOME% in PATH
	expandedPath := strings.ReplaceAll(systemPath, "%JAVA_HOME%", javaHome)

	// Update current process environment
	os.Setenv("JAVA_HOME", javaHome)

	// Get current user PATH and prepend system PATH
	userPath := os.Getenv("PATH")

	// Check if user PATH already contains the new Java path to avoid duplication
	javaBin := filepath.Join(javaHome, "bin")
	if !strings.Contains(strings.ToLower(userPath), strings.ToLower(javaBin)) {
		// Prepend new Java bin to PATH
		newPath := javaBin + ";" + userPath
		os.Setenv("PATH", newPath)
	} else {
		// Update existing PATH with expanded system PATH
		os.Setenv("PATH", expandedPath+";"+userPath)
	}

	return nil
}

// GetRefreshCommand returns a PowerShell command to refresh environment in the current session
// Users can run this command to update their current terminal without reopening it
func GetRefreshCommand() string {
	return `$env:JAVA_HOME = [System.Environment]::GetEnvironmentVariable('JAVA_HOME','Machine'); $env:Path = [System.Environment]::GetEnvironmentVariable('JAVA_HOME','Machine') + '\bin;' + $env:Path`
}

// PrintRefreshInstructions prints instructions for refreshing the current session
func PrintRefreshInstructions() {
	fmt.Println("\nTo apply changes in your current terminal session, run:")
	fmt.Println("  " + GetRefreshCommand())
	fmt.Println("\nOr simply close and reopen your terminal.")
}

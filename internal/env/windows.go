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

// GetRefreshCommand returns a PowerShell command to refresh environment in the current session
// This must be executed by the PowerShell session itself, not from within jv.exe
func GetRefreshCommand() string {
	javaHome, err := GetJavaHome()
	if err != nil {
		return ""
	}

	// Generate PowerShell command that updates the session's environment
	// This removes old Java paths and adds the new one at the front
	return fmt.Sprintf(`$env:JAVA_HOME = '%s'; $env:Path = ($env:Path -split ';' | Where-Object { $_ -notmatch '\\bin' -or $_ -notmatch 'java|jdk|jre' }) -join ';'; $env:Path = '%s\bin;' + $env:Path`,
		javaHome, javaHome)
}

// GetSimpleRefreshCommand returns a simpler PowerShell command that just prepends the new Java
func GetSimpleRefreshCommand() string {
	javaHome, err := GetJavaHome()
	if err != nil {
		return ""
	}

	return fmt.Sprintf(`$env:JAVA_HOME = '%s'; $env:Path = '%s\bin;' + $env:Path`, javaHome, javaHome)
}

// PrintRefreshInstructions prints the PowerShell command needed to refresh the current session
func PrintRefreshInstructions() {
	cmd := GetSimpleRefreshCommand()
	if cmd == "" {
		fmt.Println("\nFailed to generate refresh command. Please restart your terminal.")
		return
	}

	fmt.Println("\n" + strings.Repeat("─", 80))
	fmt.Println("To use the new Java version in this terminal, run:")
	fmt.Println()
	fmt.Println("  " + cmd)
	fmt.Println()
	fmt.Println(strings.Repeat("─", 80))
	fmt.Println("\nOr simply open a new terminal window.")
}

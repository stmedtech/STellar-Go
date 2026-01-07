package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// pathVarName returns the PATH environment variable name for the current OS
func pathVarName() string {
	if runtime.GOOS == "windows" {
		return "Path"
	}
	return "PATH"
}

// pathSeparator returns the PATH separator for the current OS
func pathSeparator() string {
	if runtime.GOOS == "windows" {
		return ";"
	}
	return ":"
}

// getPathFromEnv extracts the PATH value from an environment slice
func getPathFromEnv(env []string) string {
	pathVar := pathVarName()
	for _, envVar := range env {
		if strings.HasPrefix(envVar, pathVar+"=") {
			return strings.TrimPrefix(envVar, pathVar+"=")
		}
		// Also check uppercase PATH for cross-platform compatibility
		if strings.HasPrefix(envVar, "PATH=") {
			return strings.TrimPrefix(envVar, "PATH=")
		}
	}
	return ""
}

// setPathInEnv sets or updates the PATH in an environment slice
func setPathInEnv(env []string, pathValue string) []string {
	pathVar := pathVarName()
	// Remove existing PATH variables
	env = removeEnvVar(env, "PATH")
	env = removeEnvVar(env, "Path")
	// Add new PATH
	env = append(env, pathVar+"="+pathValue)
	return env
}

// mergePaths merges multiple PATH directories into a single PATH string
// The directories are prepended in order (first directory is searched first)
func mergePaths(dirs []string, existingPath string) string {
	if len(dirs) == 0 {
		return existingPath
	}

	sep := pathSeparator()
	var parts []string
	parts = append(parts, dirs...)
	if existingPath != "" {
		parts = append(parts, existingPath)
	}
	return strings.Join(parts, sep)
}

// findExecutableInPath searches for an executable in the given PATH
// Returns the full path to the executable if found, empty string otherwise
func findExecutableInPath(command string, pathValue string) string {
	if pathValue == "" {
		return ""
	}

	sep := pathSeparator()
	paths := strings.Split(pathValue, sep)

	for _, dir := range paths {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}

		// Build candidate path
		var candidate string
		if runtime.GOOS == "windows" {
			candidate = filepath.Join(dir, command+".exe")
		} else {
			candidate = filepath.Join(dir, command)
		}

		// Check if file exists and is executable
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			// On Unix, check if it's executable
			if runtime.GOOS != "windows" {
				if info.Mode().Perm()&0111 == 0 {
					continue
				}
			}
			return candidate
		}
	}

	return ""
}

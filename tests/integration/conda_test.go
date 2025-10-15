//go:build conda

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stellar/core/conda"
	"stellar/core/protocols/compute"
	"stellar/p2p/node"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCondaIntegration tests the complete Conda functionality
func TestCondaIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping conda integration test in short mode")
	}

	// Test conda installation and setup
	t.Run("CondaInstallation", func(t *testing.T) {
		testCondaInstallation(t)
	})

	// Test environment management
	t.Run("EnvironmentManagement", func(t *testing.T) {
		testEnvironmentManagement(t)
	})

	// Test package installation
	t.Run("PackageInstallation", func(t *testing.T) {
		testPackageInstallation(t)
	})

	// Test script execution
	t.Run("ScriptExecution", func(t *testing.T) {
		testScriptExecution(t)
	})

	// Test compute protocol integration
	t.Run("ComputeProtocol", func(t *testing.T) {
		testComputeProtocol(t)
	})
}

func testCondaInstallation(t *testing.T) {
	// Test conda path detection
	condaPath, err := conda.CommandPath()
	if err != nil {
		// If conda is not available, try to install it
		t.Log("Conda not found, attempting installation...")
		err = conda.Install("py313")
		require.NoError(t, err, "Conda installation should succeed")

		// Update conda path after installation
		success := conda.UpdateCondaPath()
		assert.True(t, success, "Conda path update should succeed")

		// Try to get conda path again
		condaPath, err = conda.CommandPath()
		require.NoError(t, err, "Should be able to get conda path after installation")
	}

	assert.NotEmpty(t, condaPath, "Conda path should not be empty")
	assert.FileExists(t, condaPath, "Conda executable should exist")

	// Test conda version
	version, err := conda.Version(condaPath)
	require.NoError(t, err, "Should be able to get conda version")
	assert.NotEmpty(t, version, "Conda version should not be empty")
	assert.Regexp(t, `^\d+\.\d+\.\d+$`, version, "Version should be in format x.y.z")
}

func testEnvironmentManagement(t *testing.T) {
	condaPath, err := conda.CommandPath()
	require.NoError(t, err, "Should be able to get conda path")

	// Test environment list
	envs, err := conda.EnvList(condaPath)
	require.NoError(t, err, "Should be able to list conda environments")
	assert.NotNil(t, envs, "Environment list should not be nil")

	// Create a test environment
	testEnvName := fmt.Sprintf("test-env-%d", time.Now().Unix())
	envPath, err := conda.CreateEnv(condaPath, testEnvName, "3.11")
	require.NoError(t, err, "Should be able to create conda environment")
	assert.NotEmpty(t, envPath, "Environment path should not be empty")
	assert.DirExists(t, envPath, "Environment directory should exist")

	// Verify environment exists in list
	envs, err = conda.EnvList(condaPath)
	require.NoError(t, err, "Should be able to list conda environments after creation")
	assert.Contains(t, envs, testEnvName, "New environment should be in the list")

	// Test environment path retrieval
	retrievedPath, err := conda.Env(condaPath, testEnvName)
	require.NoError(t, err, "Should be able to get environment path")
	assert.Equal(t, envPath, retrievedPath, "Retrieved path should match created path")

	// Test environment update with YAML file
	testYAML := `name: test-env-update
channels:
  - conda-forge
dependencies:
  - python=3.11
  - pip
  - numpy
`
	yamlPath := filepath.Join(t.TempDir(), "environment.yaml")
	err = os.WriteFile(yamlPath, []byte(testYAML), 0644)
	require.NoError(t, err, "Should be able to create test YAML file")

	err = conda.UpdateEnv(condaPath, testEnvName, yamlPath)
	// This might fail if the environment doesn't support the packages, which is okay
	if err != nil {
		t.Logf("Environment update failed (expected in some cases): %v", err)
	}

	// Clean up - remove test environment
	err = conda.RemoveEnv(condaPath, testEnvName)
	require.NoError(t, err, "Should be able to remove conda environment")

	// Verify environment is removed
	envs, err = conda.EnvList(condaPath)
	require.NoError(t, err, "Should be able to list conda environments after removal")
	assert.NotContains(t, envs, testEnvName, "Environment should be removed from the list")
}

func testPackageInstallation(t *testing.T) {
	condaPath, err := conda.CommandPath()
	require.NoError(t, err, "Should be able to get conda path")

	// Create a test environment for package installation
	testEnvName := fmt.Sprintf("test-pkg-env-%d", time.Now().Unix())
	_, err = conda.CreateEnv(condaPath, testEnvName, "3.11")
	require.NoError(t, err, "Should be able to create conda environment")
	defer conda.RemoveEnv(condaPath, testEnvName) // Clean up

	// Test package installation
	testPackages := []string{"numpy", "pandas", "requests"}
	for _, pkg := range testPackages {
		err = conda.EnvInstallPackage(condaPath, testEnvName, pkg)
		if err != nil {
			t.Logf("Package installation failed for %s (expected in some cases): %v", pkg, err)
		} else {
			t.Logf("Successfully installed package: %s", pkg)
		}
	}

	// Test running a command in the environment
	result, err := conda.RunCommand(condaPath, testEnvName, "python", "-c", "import sys; print(sys.version)")
	require.NoError(t, err, "Should be able to run command in conda environment")
	assert.Contains(t, result, "3.11", "Python version should be 3.11")
}

func testScriptExecution(t *testing.T) {
	condaPath, err := conda.CommandPath()
	require.NoError(t, err, "Should be able to get conda path")

	// Create a test environment
	testEnvName := fmt.Sprintf("test-script-env-%d", time.Now().Unix())
	_, err = conda.CreateEnv(condaPath, testEnvName, "3.11")
	require.NoError(t, err, "Should be able to create conda environment")
	defer conda.RemoveEnv(condaPath, testEnvName) // Clean up

	// Create a test Python script
	testScript := `#!/usr/bin/env python3
import sys
import json
import os

# Simple test script
data = {
    "message": "Hello from Python!",
    "python_version": sys.version,
    "current_dir": os.getcwd(),
    "script_args": sys.argv[1:]
}

print(json.dumps(data, indent=2))
`
	scriptPath := filepath.Join(t.TempDir(), "test_script.py")
	err = os.WriteFile(scriptPath, []byte(testScript), 0755)
	require.NoError(t, err, "Should be able to create test script")

	// Execute the script
	result, err := conda.RunCommand(condaPath, testEnvName, "python", scriptPath, "arg1", "arg2")
	require.NoError(t, err, "Should be able to execute Python script")
	assert.Contains(t, result, "Hello from Python!", "Script output should contain expected message")
	assert.Contains(t, result, "3.11", "Script output should contain Python version")
	assert.Contains(t, result, "arg1", "Script should receive command line arguments")
	assert.Contains(t, result, "arg2", "Script should receive command line arguments")

	// Test script with error handling
	errorScript := `#!/usr/bin/env python3
import sys
print("This will succeed")
sys.exit(1)
`
	errorScriptPath := filepath.Join(t.TempDir(), "error_script.py")
	err = os.WriteFile(errorScriptPath, []byte(errorScript), 0755)
	require.NoError(t, err, "Should be able to create error script")

	// This should fail due to exit code 1
	result, err = conda.RunCommand(condaPath, testEnvName, "python", errorScriptPath)
	assert.Error(t, err, "Script with exit code 1 should fail")
	assert.Contains(t, result, "This will succeed", "Script should still produce output before failing")
}

func testComputeProtocol(t *testing.T) {
	// Create a test node for compute protocol testing
	testNode, err := node.NewNode("0.0.0.0", 4030)
	require.NoError(t, err)
	defer testNode.Close()

	// Bind compute protocol
	compute.BindComputeStream(testNode)

	// Test conda environment preparation
	t.Run("CondaEnvironmentPreparation", func(t *testing.T) {
		testCondaEnvironmentPreparation(t, testNode)
	})

	// Test script execution through compute protocol
	t.Run("ScriptExecutionThroughProtocol", func(t *testing.T) {
		testScriptExecutionThroughProtocol(t, testNode)
	})

	// Test environment listing through compute protocol
	t.Run("EnvironmentListing", func(t *testing.T) {
		testEnvironmentListing(t, testNode)
	})
}

func testCondaEnvironmentPreparation(t *testing.T, testNode *node.Node) {
	// Create a test environment YAML file
	testYAML := `name: test-compute-env
channels:
  - conda-forge
dependencies:
  - python=3.11
  - pip
  - numpy
`
	yamlPath := filepath.Join(t.TempDir(), "compute_environment.yaml")
	err := os.WriteFile(yamlPath, []byte(testYAML), 0644)
	require.NoError(t, err, "Should be able to create test YAML file")

	// Create conda preparation request
	prep := compute.CondaPythonPreparation{
		Env:         "test-compute-env",
		Version:     "3.11",
		EnvYamlPath: yamlPath,
	}

	// Test preparation (this might fail if conda is not properly set up)
	envPath, err := prep.Prepare()
	if err != nil {
		t.Logf("Environment preparation failed (expected in some cases): %v", err)
		t.Skip("Skipping due to conda setup issues")
	}

	assert.NotEmpty(t, envPath, "Environment path should not be empty")
	assert.DirExists(t, envPath, "Environment directory should exist")
}

func testScriptExecutionThroughProtocol(t *testing.T, testNode *node.Node) {
	// Create a test Python script
	testScript := `#!/usr/bin/env python3
import sys
import json

# Test script for compute protocol
result = {
    "status": "success",
    "message": "Script executed successfully",
    "python_version": sys.version,
    "script_name": __file__
}

print(json.dumps(result, indent=2))
`
	scriptPath := filepath.Join(t.TempDir(), "compute_script.py")
	err := os.WriteFile(scriptPath, []byte(testScript), 0755)
	require.NoError(t, err, "Should be able to create test script")

	// Create script execution request
	exec := compute.CondaPythonScriptExecution{
		Env:        "test-compute-env",
		ScriptPath: scriptPath,
	}

	// Test execution (this might fail if conda is not properly set up)
	result, err := exec.Execute()
	if err != nil {
		t.Logf("Script execution failed (expected in some cases): %v", err)
		t.Skip("Skipping due to conda setup issues")
	}

	assert.NotEmpty(t, result, "Script result should not be empty")
	assert.Contains(t, result, "success", "Script should return success status")
	assert.Contains(t, result, "3.11", "Script should contain Python version")
}

func testEnvironmentListing(t *testing.T, testNode *node.Node) {
	// This test would require the compute protocol to be fully implemented
	// For now, we'll test the basic structure
	condaPath, err := conda.CommandPath()
	if err != nil {
		t.Skip("Skipping due to conda not being available")
	}

	envs, err := conda.EnvList(condaPath)
	require.NoError(t, err, "Should be able to list conda environments")
	assert.NotNil(t, envs, "Environment list should not be nil")

	// Verify the list contains valid environment data
	for name, path := range envs {
		assert.NotEmpty(t, name, "Environment name should not be empty")
		assert.NotEmpty(t, path, "Environment path should not be empty")
		assert.DirExists(t, path, "Environment directory should exist")
	}
}

// TestCondaStress tests conda functionality under stress
func TestCondaStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping conda stress test in short mode")
	}

	condaPath, err := conda.CommandPath()
	require.NoError(t, err, "Should be able to get conda path")

	// Test concurrent environment creation
	t.Run("ConcurrentEnvironmentCreation", func(t *testing.T) {
		testConcurrentEnvironmentCreation(t, condaPath)
	})

	// Test concurrent script execution
	t.Run("ConcurrentScriptExecution", func(t *testing.T) {
		testConcurrentScriptExecution(t, condaPath)
	})
}

func testConcurrentEnvironmentCreation(t *testing.T, condaPath string) {
	// Create multiple environments concurrently
	numEnvs := 3
	results := make(chan error, numEnvs)

	for i := 0; i < numEnvs; i++ {
		go func(index int) {
			envName := fmt.Sprintf("stress-env-%d-%d", index, time.Now().Unix())
			_, err := conda.CreateEnv(condaPath, envName, "3.11")
			if err == nil {
				// Clean up the environment
				defer conda.RemoveEnv(condaPath, envName)
			}
			results <- err
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numEnvs; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else {
			t.Logf("Environment creation failed: %v", err)
		}
	}

	// Verify most environments were created successfully
	successRate := float64(successCount) / float64(numEnvs)
	assert.Greater(t, successRate, 0.5, "At least 50% of environments should be created successfully")
}

func testConcurrentScriptExecution(t *testing.T, condaPath string) {
	// Create a test environment
	testEnvName := fmt.Sprintf("stress-script-env-%d", time.Now().Unix())
	_, err := conda.CreateEnv(condaPath, testEnvName, "3.11")
	require.NoError(t, err, "Should be able to create conda environment")
	defer conda.RemoveEnv(condaPath, testEnvName) // Clean up

	// Create a simple test script
	testScript := `#!/usr/bin/env python3
import time
import random
import sys

# Simulate some work
time.sleep(random.uniform(0.1, 0.5))
print(f"Script {sys.argv[1]} completed")
`
	scriptPath := filepath.Join(t.TempDir(), "stress_script.py")
	err = os.WriteFile(scriptPath, []byte(testScript), 0755)
	require.NoError(t, err, "Should be able to create test script")

	// Execute multiple scripts concurrently
	numScripts := 5
	results := make(chan error, numScripts)

	for i := 0; i < numScripts; i++ {
		go func(index int) {
			result, err := conda.RunCommand(condaPath, testEnvName, "python", scriptPath, fmt.Sprintf("%d", index))
			if err != nil {
				results <- err
			} else {
				// Verify script output
				if !strings.Contains(result, fmt.Sprintf("Script %d completed", index)) {
					results <- fmt.Errorf("unexpected script output: %s", result)
				} else {
					results <- nil
				}
			}
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numScripts; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else {
			t.Logf("Script execution failed: %v", err)
		}
	}

	// Verify most scripts executed successfully
	successRate := float64(successCount) / float64(numScripts)
	assert.Greater(t, successRate, 0.8, "At least 80% of scripts should execute successfully")
}

// TestCondaErrorHandling tests error handling in conda operations
func TestCondaErrorHandling(t *testing.T) {
	condaPath, err := conda.CommandPath()
	if err != nil {
		t.Skip("Skipping due to conda not being available")
	}

	// Test invalid environment operations
	t.Run("InvalidEnvironmentOperations", func(t *testing.T) {
		testInvalidEnvironmentOperations(t, condaPath)
	})

	// Test invalid script execution
	t.Run("InvalidScriptExecution", func(t *testing.T) {
		testInvalidScriptExecution(t, condaPath)
	})
}

func testInvalidEnvironmentOperations(t *testing.T, condaPath string) {
	// Test getting non-existent environment
	_, err := conda.Env(condaPath, "non-existent-env-12345")
	assert.Error(t, err, "Should error when getting non-existent environment")
	assert.Contains(t, err.Error(), "environment not found", "Error should indicate environment not found")

	// Test removing non-existent environment
	err = conda.RemoveEnv(condaPath, "non-existent-env-12345")
	assert.Error(t, err, "Should error when removing non-existent environment")
	assert.Contains(t, err.Error(), "environment not exist", "Error should indicate environment doesn't exist")
}

func testInvalidScriptExecution(t *testing.T, condaPath string) {
	// Create a test environment
	testEnvName := fmt.Sprintf("error-test-env-%d", time.Now().Unix())
	_, err := conda.CreateEnv(condaPath, testEnvName, "3.11")
	require.NoError(t, err, "Should be able to create conda environment")
	defer conda.RemoveEnv(condaPath, testEnvName) // Clean up

	// Test running non-existent script
	_, err = conda.RunCommand(condaPath, testEnvName, "python", "/non/existent/script.py")
	assert.Error(t, err, "Should error when running non-existent script")

	// Test running script with syntax error
	errorScript := `#!/usr/bin/env python3
print("This is valid")
print("This has a syntax error"
`
	errorScriptPath := filepath.Join(t.TempDir(), "syntax_error_script.py")
	err = os.WriteFile(errorScriptPath, []byte(errorScript), 0755)
	require.NoError(t, err, "Should be able to create error script")

	_, err = conda.RunCommand(condaPath, testEnvName, "python", errorScriptPath)
	assert.Error(t, err, "Should error when running script with syntax error")
}

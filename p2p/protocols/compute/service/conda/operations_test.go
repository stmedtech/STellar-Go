package conda

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnvListOutput_Empty(t *testing.T) {
	output := "# conda environments:\n#\n"
	envs, err := ParseEnvListOutput(output)
	require.NoError(t, err)
	assert.Empty(t, envs, "should return empty map when no environments")
}

func TestParseEnvListOutput_BaseOnly(t *testing.T) {
	output := "# conda environments:\n#\nbase                  /opt/conda\n"
	envs, err := ParseEnvListOutput(output)
	require.NoError(t, err)
	assert.Len(t, envs, 1, "should include base environment")
	assert.Equal(t, "/opt/conda", envs["base"])
}

func TestParseEnvListOutput_SingleEnv(t *testing.T) {
	output := "# conda environments:\n#\nbase                  /opt/conda\ntestenv               /opt/conda/envs/testenv\n"
	envs, err := ParseEnvListOutput(output)
	require.NoError(t, err)
	assert.Len(t, envs, 2, "should include both base and testenv")
	assert.Equal(t, "/opt/conda", envs["base"])
	assert.Equal(t, "/opt/conda/envs/testenv", envs["testenv"])
}

func TestParseEnvListOutput_MultipleEnvs(t *testing.T) {
	output := "# conda environments:\n#\nbase                  /opt/conda\ntestenv1              /opt/conda/envs/testenv1\ntestenv2              /opt/conda/envs/testenv2\n"
	envs, err := ParseEnvListOutput(output)
	require.NoError(t, err)
	assert.Len(t, envs, 3, "should include base, testenv1, and testenv2")
	assert.Equal(t, "/opt/conda", envs["base"])
	assert.Equal(t, "/opt/conda/envs/testenv1", envs["testenv1"])
	assert.Equal(t, "/opt/conda/envs/testenv2", envs["testenv2"])
}

func TestParseEnvListOutput_ActiveEnv(t *testing.T) {
	output := "# conda environments:\n#\nbase                  /opt/conda\n*testenv              /opt/conda/envs/testenv\n"
	envs, err := ParseEnvListOutput(output)
	require.NoError(t, err)
	assert.Len(t, envs, 2, "should include both base and testenv")
	assert.Equal(t, "/opt/conda", envs["base"])
	assert.Equal(t, "/opt/conda/envs/testenv", envs["testenv"], "should strip asterisk from active env")
}

func TestParseEnvListOutput_InvalidOutput(t *testing.T) {
	output := "invalid output format"
	envs, err := ParseEnvListOutput(output)
	require.NoError(t, err)
	assert.Empty(t, envs, "should return empty map for invalid format")
}

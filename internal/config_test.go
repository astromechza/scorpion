package internal

import (
	"github.com/score-spec/score-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestLoadConfig_missing(t *testing.T) {
	t.Chdir(t.TempDir())
	c, ok, err := LoadConfig()
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, ScoreConfig{}, c)
}

func TestLoadConfig_bad_file(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir(ConfigFile, 0o700))
	c, ok, err := LoadConfig()
	require.EqualError(t, err, "failed to decode config file: read config.json: is a directory")
	require.False(t, ok)
	require.Equal(t, ScoreConfig{}, c)
}

func TestLoadConfig_bad_content(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile(ConfigFile, []byte(`{"workloads":[],"blobs":4}`), 0o644))
	c, ok, err := LoadConfig()
	require.EqualError(t, err, "failed to decode config file: json: unknown field \"blobs\"")
	require.False(t, ok)
	require.Equal(t, ScoreConfig{}, c)
}

func TestLoadConfig_good(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile(ConfigFile, []byte(`{"workloads":[{"apiVersion":"score.dev/v1b1","metadata":{"name":"app"},"containers":{"main":{"image":"thing"}}}]}`), 0o644))
	c, ok, err := LoadConfig()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, ScoreConfig{Workloads: []types.Workload{{
		ApiVersion: "score.dev/v1b1",
		Metadata: map[string]interface{}{
			"name": "app",
		},
		Containers: map[string]types.Container{
			"main": {Image: "thing"},
		},
	}}}, c)
}

func TestSaveConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := ScoreConfig{Workloads: []types.Workload{{
		ApiVersion: "score.dev/v1b1",
		Metadata: map[string]interface{}{
			"name": "app",
		},
		Containers: map[string]types.Container{
			"main": {Image: "thing"},
		},
	}}}
	require.NoError(t, SaveConfig(cfg))
	cfg2, ok, err := LoadConfig()
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, cfg, cfg2)
	_, err = os.Stat(ConfigFile + ".tmp")
	require.EqualError(t, err, "stat config.json.tmp: no such file or directory")
}

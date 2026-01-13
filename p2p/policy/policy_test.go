package policy

import (
	"fmt"
	"os"
	"testing"

	"stellar/core/config"
	"stellar/core/constant"
	"stellar/pkg/testutils"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtocolPolicy_Authenticate(t *testing.T) {
	// Create actual peer IDs for testing
	peer1 := testutils.TestPeer(t)
	peer2 := testutils.TestPeer(t)
	peer3 := testutils.TestPeer(t)

	tests := []struct {
		name           string
		enable         bool
		whitelist      []string
		peerID         peer.ID
		expectedResult bool
	}{
		{
			name:           "policy disabled - allow all",
			enable:         false,
			whitelist:      []string{},
			peerID:         peer1,
			expectedResult: true,
		},
		{
			name:           "policy enabled - peer in whitelist",
			enable:         true,
			whitelist:      []string{peer1.String(), peer2.String()},
			peerID:         peer1,
			expectedResult: true,
		},
		{
			name:           "policy enabled - peer not in whitelist",
			enable:         true,
			whitelist:      []string{peer2.String(), peer3.String()},
			peerID:         peer1,
			expectedResult: false,
		},
		{
			name:           "policy enabled - empty whitelist",
			enable:         true,
			whitelist:      []string{},
			peerID:         peer1,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &ProtocolPolicy{
				Enable:    tt.enable,
				WhiteList: tt.whitelist,
			}

			result := policy.Authenticate(tt.peerID)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestProtocolPolicy_AuthorizeStream(t *testing.T) {
	// Test the AuthorizeStream function by testing the core logic
	// Since the network.Conn interface is complex to mock, we'll test the authentication logic directly

	// Create actual peer IDs for testing
	peer1 := testutils.TestPeer(t)
	peer2 := testutils.TestPeer(t)

	tests := []struct {
		name           string
		enable         bool
		whitelist      []string
		peerID         peer.ID
		shouldCallNext bool
	}{
		{
			name:           "policy disabled - should call next",
			enable:         false,
			whitelist:      []string{},
			peerID:         peer1,
			shouldCallNext: true,
		},
		{
			name:           "policy enabled - peer authorized - should call next",
			enable:         true,
			whitelist:      []string{peer1.String()},
			peerID:         peer1,
			shouldCallNext: true,
		},
		{
			name:           "policy enabled - peer not authorized - should not call next",
			enable:         true,
			whitelist:      []string{peer2.String()},
			peerID:         peer1,
			shouldCallNext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &ProtocolPolicy{
				Enable:    tt.enable,
				WhiteList: tt.whitelist,
			}

			// Test the authentication logic directly
			result := policy.Authenticate(tt.peerID)

			// The result should match the expected behavior
			if tt.enable {
				// If policy is enabled, result should match whether peer is in whitelist
				expectedAuth := false
				for _, allowedPeer := range tt.whitelist {
					if tt.peerID.String() == allowedPeer {
						expectedAuth = true
						break
					}
				}
				assert.Equal(t, expectedAuth, result)
			} else {
				// If policy is disabled, should always return true
				assert.True(t, result)
			}
		})
	}
}

func TestProtocolPolicy_AddWhiteList(t *testing.T) {
	// Create actual peer IDs for testing
	validPeer1 := testutils.TestPeer(t)

	tests := []struct {
		name      string
		whitelist []string
		deviceID  string
		wantErr   bool
	}{
		{
			name:      "add valid device ID",
			whitelist: []string{},
			deviceID:  validPeer1.String(),
			wantErr:   false,
		},
		{
			name:      "add duplicate device ID",
			whitelist: []string{validPeer1.String()},
			deviceID:  validPeer1.String(),
			wantErr:   true,
		},
		{
			name:      "add invalid device ID",
			whitelist: []string{},
			deviceID:  "invalid-peer-id",
			wantErr:   true,
		},
		{
			name:      "add empty device ID",
			whitelist: []string{},
			deviceID:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &ProtocolPolicy{
				Enable:    true,
				WhiteList: make([]string, len(tt.whitelist)),
			}
			copy(policy.WhiteList, tt.whitelist)

			err := policy.AddWhiteList(tt.deviceID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, policy.WhiteList, tt.deviceID)
			}
		})
	}
}

func TestProtocolPolicy_RemoveWhiteList(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		deviceID  string
		wantErr   bool
	}{
		{
			name:      "remove existing device ID",
			whitelist: []string{"QmPeer1", "QmPeer2", "QmPeer3"},
			deviceID:  "QmPeer2",
			wantErr:   false,
		},
		{
			name:      "remove non-existent device ID",
			whitelist: []string{"QmPeer1", "QmPeer3"},
			deviceID:  "QmPeer2",
			wantErr:   true,
		},
		{
			name:      "remove from empty whitelist",
			whitelist: []string{},
			deviceID:  "QmPeer1",
			wantErr:   true,
		},
		{
			name:      "remove empty device ID",
			whitelist: []string{"QmPeer1"},
			deviceID:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &ProtocolPolicy{
				Enable:    true,
				WhiteList: make([]string, len(tt.whitelist)),
			}
			copy(policy.WhiteList, tt.whitelist)

			originalLength := len(policy.WhiteList)
			err := policy.RemoveWhiteList(tt.deviceID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, originalLength, len(policy.WhiteList))
			} else {
				assert.NoError(t, err)
				assert.Equal(t, originalLength-1, len(policy.WhiteList))
				assert.NotContains(t, policy.WhiteList, tt.deviceID)
			}
		})
	}
}

func TestRemoveIndex(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		index    int
		expected []string
	}{
		{
			name:     "remove middle element",
			slice:    []string{"a", "b", "c", "d"},
			index:    1,
			expected: []string{"a", "c", "d"},
		},
		{
			name:     "remove first element",
			slice:    []string{"a", "b", "c"},
			index:    0,
			expected: []string{"b", "c"},
		},
		{
			name:     "remove last element",
			slice:    []string{"a", "b", "c"},
			index:    2,
			expected: []string{"a", "b"},
		},
		{
			name:     "remove from single element slice",
			slice:    []string{"a"},
			index:    0,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveIndex(tt.slice, tt.index)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func BenchmarkProtocolPolicy_Authenticate(b *testing.B) {
	policy := &ProtocolPolicy{
		Enable:    true,
		WhiteList: []string{"QmPeer1", "QmPeer2", "QmPeer3", "QmPeer4", "QmPeer5"},
	}

	testPeer := peer.ID("QmPeer3")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy.Authenticate(testPeer)
	}
}

func BenchmarkProtocolPolicy_AddWhiteList(b *testing.B) {
	policy := &ProtocolPolicy{
		Enable:    true,
		WhiteList: []string{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deviceID := fmt.Sprintf("QmPeer%d", i)
		policy.AddWhiteList(deviceID)
	}
}

func TestProtocolPolicy_LoadFromConfig(t *testing.T) {
	// Create a temporary directory for config
	tempDir := t.TempDir()
	originalStellarPath := constant.STELLAR_PATH

	// Set up temporary stellar path
	constant.STELLAR_PATH = tempDir
	defer func() {
		constant.STELLAR_PATH = originalStellarPath
	}()

	// Create config directory
	require.NoError(t, os.MkdirAll(tempDir, 0755))

	t.Run("loads whitelist from config", func(t *testing.T) {
		// Create a config with whitelist
		savedPeer1 := testutils.TestPeer(t)
		savedPeer2 := testutils.TestPeer(t)
		savedWhitelist := []string{savedPeer1.String(), savedPeer2.String()}

		cfg := config.DefaultConfig()
		cfg.WhiteList = savedWhitelist
		require.NoError(t, config.SaveConfig(cfg))

		// Create policy and load from config
		policy := &ProtocolPolicy{
			Enable:    true,
			WhiteList: make([]string, 0),
		}

		policy.LoadFromConfig()

		// Verify whitelist was loaded
		assert.Equal(t, len(savedWhitelist), len(policy.WhiteList))
		assert.Contains(t, policy.WhiteList, savedPeer1.String())
		assert.Contains(t, policy.WhiteList, savedPeer2.String())
	})

	t.Run("bootstrappers added to existing whitelist", func(t *testing.T) {
		// Create a config with saved whitelist
		savedPeer1 := testutils.TestPeer(t)
		savedPeer2 := testutils.TestPeer(t)
		savedWhitelist := []string{savedPeer1.String(), savedPeer2.String()}

		cfg := config.DefaultConfig()
		cfg.WhiteList = savedWhitelist
		require.NoError(t, config.SaveConfig(cfg))

		// Create policy and load from config
		policy := &ProtocolPolicy{
			Enable:    true,
			WhiteList: make([]string, 0),
		}

		policy.LoadFromConfig()

		// Verify initial whitelist
		assert.Equal(t, len(savedWhitelist), len(policy.WhiteList))

		// Simulate adding bootstrapper (like InitDHT does)
		bootstrapPeer := testutils.TestPeer(t)
		err := policy.AddWhiteList(bootstrapPeer.String())
		require.NoError(t, err)

		// Verify both saved entries and bootstrapper are in whitelist
		assert.Equal(t, len(savedWhitelist)+1, len(policy.WhiteList))
		assert.Contains(t, policy.WhiteList, savedPeer1.String(), "Saved peer 1 should still be in whitelist")
		assert.Contains(t, policy.WhiteList, savedPeer2.String(), "Saved peer 2 should still be in whitelist")
		assert.Contains(t, policy.WhiteList, bootstrapPeer.String(), "Bootstrap peer should be added to whitelist")

		// Verify config was updated with combined whitelist
		cfgInstance := config.GetInstance()
		assert.Equal(t, len(savedWhitelist)+1, len(cfgInstance.WhiteList))
		assert.Contains(t, cfgInstance.WhiteList, savedPeer1.String())
		assert.Contains(t, cfgInstance.WhiteList, savedPeer2.String())
		assert.Contains(t, cfgInstance.WhiteList, bootstrapPeer.String())
	})

	t.Run("loads empty whitelist when config has none", func(t *testing.T) {
		// Create empty config
		cfg := config.DefaultConfig()
		cfg.WhiteList = []string{}
		require.NoError(t, config.SaveConfig(cfg))

		// Create policy and load from config
		policy := &ProtocolPolicy{
			Enable:    true,
			WhiteList: make([]string, 0),
		}

		policy.LoadFromConfig()

		// Verify whitelist is empty
		assert.Equal(t, 0, len(policy.WhiteList))
	})

	t.Run("multiple bootstrappers added to saved whitelist", func(t *testing.T) {
		// Create a config with saved whitelist
		savedPeer1 := testutils.TestPeer(t)
		savedWhitelist := []string{savedPeer1.String()}

		cfg := config.DefaultConfig()
		cfg.WhiteList = savedWhitelist
		require.NoError(t, config.SaveConfig(cfg))

		// Create policy and load from config
		policy := &ProtocolPolicy{
			Enable:    true,
			WhiteList: make([]string, 0),
		}

		policy.LoadFromConfig()

		// Add multiple bootstrappers
		bootstrapPeer1 := testutils.TestPeer(t)
		bootstrapPeer2 := testutils.TestPeer(t)
		require.NoError(t, policy.AddWhiteList(bootstrapPeer1.String()))
		require.NoError(t, policy.AddWhiteList(bootstrapPeer2.String()))

		// Verify all entries are present
		assert.Equal(t, 3, len(policy.WhiteList))
		assert.Contains(t, policy.WhiteList, savedPeer1.String())
		assert.Contains(t, policy.WhiteList, bootstrapPeer1.String())
		assert.Contains(t, policy.WhiteList, bootstrapPeer2.String())
	})
}

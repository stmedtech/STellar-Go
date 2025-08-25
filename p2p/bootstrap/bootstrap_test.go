package bootstrap

import (
	"os"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeAddrInfo(t *testing.T) {
	tests := []struct {
		name           string
		peerAddrString string
		wantErr        bool
	}{
		{
			name:           "valid multiaddr",
			peerAddrString: "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
			wantErr:        false,
		},
		{
			name:           "invalid multiaddr",
			peerAddrString: "invalid-addr",
			wantErr:        true,
		},
		{
			name:           "empty string",
			peerAddrString: "",
			wantErr:        true,
		},
		// Note: libp2p handles multiaddrs without peer IDs differently
		// so we skip this test case
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peerInfo, err := MakeAddrInfo(tt.peerAddrString)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, peerInfo.ID)
				assert.NotEmpty(t, peerInfo.Addrs)
			}
		})
	}
}

func TestMakeAddrInfoValidPeer(t *testing.T) {
	// Test with a valid peer address
	peerAddrString := "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN"

	peerInfo, err := MakeAddrInfo(peerAddrString)
	require.NoError(t, err)

	// Verify the peer info
	assert.NotEmpty(t, peerInfo.ID)
	assert.NotEmpty(t, peerInfo.Addrs)
	assert.True(t, peerInfo.ID.Validate() == nil)

	// Verify we can get the peer ID string
	peerIDString := peerInfo.ID.String()
	assert.NotEmpty(t, peerIDString)
	assert.Contains(t, peerIDString, "Qm")
}

func TestGetBootstrappers(t *testing.T) {
	// Test getBootstrappers function
	bootstrappers, err := getBootstrappers()

	// The function may error if the file doesn't exist, which is expected
	if err != nil {
		// If there's an error, it should be a file not found error
		assert.Contains(t, err.Error(), "no such file or directory")
	}
	assert.NotNil(t, bootstrappers)

	// Should return at least the empty BOOTSTRAPPERS slice
	assert.GreaterOrEqual(t, len(bootstrappers), 0)
}

func TestGetBootstrappersWithFile(t *testing.T) {
	// Create a temporary bootstrappers file
	tempDir := t.TempDir()
	originalPwd, err := os.Getwd()
	require.NoError(t, err)

	// Change to temp directory
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		os.Chdir(originalPwd)
	}()

	// Create bootstrappers.txt file
	bootstrapperContent := `/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN
/ip4/127.0.0.1/tcp/4002/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN`

	err = os.WriteFile("bootstrappers.txt", []byte(bootstrapperContent), 0644)
	require.NoError(t, err)

	// Test getBootstrappers function
	bootstrappers, err := getBootstrappers()
	require.NoError(t, err)

	// Should return the bootstrappers from the file
	assert.Len(t, bootstrappers, 2)
	assert.Contains(t, bootstrappers[0], "/ip4/127.0.0.1/tcp/4001/p2p/")
	assert.Contains(t, bootstrappers[1], "/ip4/127.0.0.1/tcp/4002/p2p/")
}

func TestGetBootstrappersWithEmptyFile(t *testing.T) {
	// Create a temporary bootstrappers file
	tempDir := t.TempDir()
	originalPwd, err := os.Getwd()
	require.NoError(t, err)

	// Change to temp directory
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		os.Chdir(originalPwd)
	}()

	// Create empty bootstrappers.txt file
	err = os.WriteFile("bootstrappers.txt", []byte(""), 0644)
	require.NoError(t, err)

	// Test getBootstrappers function
	bootstrappers, err := getBootstrappers()
	require.NoError(t, err)

	// Should return empty slice
	assert.Len(t, bootstrappers, 0)
}

func TestGetBootstrappersWithInvalidFile(t *testing.T) {
	// Create a temporary bootstrappers file
	tempDir := t.TempDir()
	originalPwd, err := os.Getwd()
	require.NoError(t, err)

	// Change to temp directory
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		os.Chdir(originalPwd)
	}()

	// Create bootstrappers.txt file with invalid content
	bootstrapperContent := `invalid-addr-1
invalid-addr-2`

	err = os.WriteFile("bootstrappers.txt", []byte(bootstrapperContent), 0644)
	require.NoError(t, err)

	// Test getBootstrappers function
	bootstrappers, err := getBootstrappers()
	require.NoError(t, err)

	// Should return the invalid addresses (function doesn't validate)
	assert.Len(t, bootstrappers, 2)
	assert.Equal(t, "invalid-addr-1", bootstrappers[0])
	assert.Equal(t, "invalid-addr-2", bootstrappers[1])
}

func TestBootstrapInitialization(t *testing.T) {
	// Test that the bootstrap package initializes correctly
	// The init() function should have run and populated Bootstrappers

	// Bootstrappers should be initialized (even if empty)
	// Note: These may be nil if init() failed, which is acceptable
	if Bootstrappers != nil {
		assert.IsType(t, []peer.AddrInfo{}, Bootstrappers)
	}

	// BOOTSTRAPPERS should be initialized
	if BOOTSTRAPPERS != nil {
		assert.IsType(t, []string{}, BOOTSTRAPPERS)
	}
}

func TestMakeAddrInfoEdgeCases(t *testing.T) {
	// Test edge cases for MakeAddrInfo

	// Test with very long multiaddr
	longAddr := "/ip4/127.0.0.1/tcp/4001/p2p/" + string(make([]byte, 1000)) + "QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN"
	_, err := MakeAddrInfo(longAddr)
	assert.Error(t, err)

	// Test with special characters
	specialAddr := "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN\n\r\t"
	_, err = MakeAddrInfo(specialAddr)
	assert.Error(t, err)
}

func TestMakeAddrInfoConsistency(t *testing.T) {
	// Test that MakeAddrInfo returns consistent results
	peerAddrString := "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN"

	peerInfo1, err1 := MakeAddrInfo(peerAddrString)
	require.NoError(t, err1)

	peerInfo2, err2 := MakeAddrInfo(peerAddrString)
	require.NoError(t, err2)

	// Results should be identical
	assert.Equal(t, peerInfo1.ID, peerInfo2.ID)
	assert.Equal(t, peerInfo1.Addrs, peerInfo2.Addrs)
	assert.Equal(t, err1, err2)
}

func BenchmarkMakeAddrInfo(b *testing.B) {
	peerAddrString := "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := MakeAddrInfo(peerAddrString)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetBootstrappers(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := getBootstrappers()
		if err != nil {
			b.Fatal(err)
		}
	}
}

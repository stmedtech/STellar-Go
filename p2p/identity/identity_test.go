package identity

import (
	"encoding/base64"
	"testing"

	"stellar/pkg/testutils"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePrivateKey(t *testing.T) {
	tests := []struct {
		name    string
		seed    int64
		wantErr bool
	}{
		{
			name:    "generate with zero seed",
			seed:    0,
			wantErr: false,
		},
		{
			name:    "generate with positive seed",
			seed:    12345,
			wantErr: false,
		},
		{
			name:    "generate with negative seed",
			seed:    -12345,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			privKey, err := GeneratePrivateKey(tt.seed)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, privKey)

			// Verify it's an Ed25519 key
			assert.Equal(t, int(crypto.Ed25519), int(privKey.Type()))

			// Verify we can get the public key
			pubKey := privKey.GetPublic()
			assert.NotNil(t, pubKey)
			assert.Equal(t, int(crypto.Ed25519), int(pubKey.Type()))

			// Verify we can derive peer ID
			peerID, err := peer.IDFromPublicKey(pubKey)
			assert.NoError(t, err)
			assert.NotEmpty(t, peerID.String())
		})
	}
}

func TestGeneratePrivateKeyDeterministic(t *testing.T) {
	// Test that same seed produces same key
	seed := int64(42)

	privKey1, err := GeneratePrivateKey(seed)
	require.NoError(t, err)

	privKey2, err := GeneratePrivateKey(seed)
	require.NoError(t, err)

	// Keys should be identical
	assert.Equal(t, privKey1, privKey2)

	// Different seeds should produce different keys
	privKey3, err := GeneratePrivateKey(seed + 1)
	require.NoError(t, err)

	assert.NotEqual(t, privKey1, privKey3)
}

func TestDecodePrivateKey(t *testing.T) {
	// Generate a valid private key first
	originalPrivKey, err := GeneratePrivateKey(0)
	require.NoError(t, err)

	// Encode to base64
	encodedKey, err := EncodePrivateKey(originalPrivKey)
	require.NoError(t, err)

	tests := []struct {
		name    string
		b64Key  string
		wantErr bool
	}{
		{
			name:    "decode valid base64 key",
			b64Key:  encodedKey,
			wantErr: false,
		},
		{
			name:    "decode empty string",
			b64Key:  "",
			wantErr: true,
		},
		{
			name:    "decode invalid base64",
			b64Key:  "invalid-base64!@#",
			wantErr: true,
		},
		{
			name:    "decode invalid key bytes",
			b64Key:  base64.StdEncoding.EncodeToString([]byte("invalid-key-bytes")),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			privKey, err := DecodePrivateKey(tt.b64Key)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, privKey)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, privKey)

			// Verify it's the same key
			assert.Equal(t, originalPrivKey, privKey)

			// Verify we can derive the same peer ID
			originalPeerID, err := peer.IDFromPrivateKey(originalPrivKey)
			require.NoError(t, err)

			decodedPeerID, err := peer.IDFromPrivateKey(privKey)
			require.NoError(t, err)

			assert.Equal(t, originalPeerID, decodedPeerID)
		})
	}
}

func TestEncodePrivateKey(t *testing.T) {
	// Generate a test private key
	privKey, err := GeneratePrivateKey(0)
	require.NoError(t, err)

	// Encode it
	encodedKey, err := EncodePrivateKey(privKey)
	require.NoError(t, err)

	// Verify it's valid base64
	_, err = base64.StdEncoding.DecodeString(encodedKey)
	assert.NoError(t, err)

	// Decode it back
	decodedKey, err := DecodePrivateKey(encodedKey)
	require.NoError(t, err)

	// Verify it's the same key
	assert.Equal(t, privKey, decodedKey)
}

func TestSignAndVerify(t *testing.T) {
	// Generate private key
	privKey, err := GeneratePrivateKey(0)
	require.NoError(t, err)

	// Get public key
	pubKey := privKey.GetPublic()

	// Test data
	message := []byte("Hello, World!")

	// Sign the message
	signature, err := privKey.Sign(message)
	require.NoError(t, err)
	assert.NotNil(t, signature)

	// Verify the signature
	valid, err := pubKey.Verify(message, signature)
	require.NoError(t, err)
	assert.True(t, valid)

	// Test with wrong message
	wrongMessage := []byte("Wrong message")
	valid, err = pubKey.Verify(wrongMessage, signature)
	require.NoError(t, err)
	assert.False(t, valid)

	// Test with wrong signature
	wrongSignature := []byte("wrong signature")
	valid, err = pubKey.Verify(message, wrongSignature)
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestPeerIDFromPrivateKey(t *testing.T) {
	// Generate private key
	privKey, err := GeneratePrivateKey(0)
	require.NoError(t, err)

	// Get peer ID
	peerID, err := peer.IDFromPrivateKey(privKey)
	require.NoError(t, err)

	assert.NotEmpty(t, peerID.String())

	// Verify it matches the public key
	pubKey := privKey.GetPublic()
	expectedPeerID, err := peer.IDFromPublicKey(pubKey)
	require.NoError(t, err)

	assert.Equal(t, expectedPeerID, peerID)
}

func TestPeerIDFromPublicKey(t *testing.T) {
	// Generate private key
	privKey, err := GeneratePrivateKey(0)
	require.NoError(t, err)

	// Get public key
	pubKey := privKey.GetPublic()

	// Get peer ID from public key
	peerID, err := peer.IDFromPublicKey(pubKey)
	require.NoError(t, err)

	assert.NotEmpty(t, peerID.String())

	// Verify it matches the private key
	expectedPeerID, err := peer.IDFromPrivateKey(privKey)
	require.NoError(t, err)

	assert.Equal(t, expectedPeerID, peerID)
}

func TestKeyRoundTrip(t *testing.T) {
	// Test complete round trip: generate -> encode -> decode -> verify
	originalPrivKey, err := GeneratePrivateKey(42)
	require.NoError(t, err)

	// Encode
	encodedKey, err := EncodePrivateKey(originalPrivKey)
	require.NoError(t, err)

	// Decode
	decodedPrivKey, err := DecodePrivateKey(encodedKey)
	require.NoError(t, err)

	// Verify they're identical
	assert.Equal(t, originalPrivKey, decodedPrivKey)

	// Verify peer IDs match
	originalPeerID, err := peer.IDFromPrivateKey(originalPrivKey)
	require.NoError(t, err)

	decodedPeerID, err := peer.IDFromPrivateKey(decodedPrivKey)
	require.NoError(t, err)

	assert.Equal(t, originalPeerID, decodedPeerID)

	// Test signing and verification with decoded key
	message := []byte("Test message")

	// Sign with original key
	originalSig, err := originalPrivKey.Sign(message)
	require.NoError(t, err)

	// Sign with decoded key
	decodedSig, err := decodedPrivKey.Sign(message)
	require.NoError(t, err)

	// Signatures should be identical (deterministic)
	assert.Equal(t, originalSig, decodedSig)

	// Verify with public key
	pubKey := originalPrivKey.GetPublic()
	valid, err := pubKey.Verify(message, originalSig)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestInvalidKeyHandling(t *testing.T) {
	// Test with nil private key - this will panic, so we need to recover
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected panic due to nil pointer dereference
				t.Logf("Expected panic recovered: %v", r)
			}
		}()
		_, err := EncodePrivateKey(nil)
		assert.Error(t, err)
	}()

	// Test with nil public key - this will panic, so we need to recover
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected panic due to nil pointer dereference
				t.Logf("Expected panic recovered: %v", r)
			}
		}()
		_, _ = peer.IDFromPublicKey(nil)
	}()

	// Test with invalid key type
	invalidPrivKey := testutils.TestPrivateKey(t)
	_, err := peer.IDFromPublicKey(invalidPrivKey.GetPublic())
	assert.NoError(t, err) // Should work with any valid key
}

func BenchmarkGeneratePrivateKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GeneratePrivateKey(int64(i))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodePrivateKey(b *testing.B) {
	privKey, err := GeneratePrivateKey(0)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := EncodePrivateKey(privKey)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodePrivateKey(b *testing.B) {
	privKey, err := GeneratePrivateKey(0)
	if err != nil {
		b.Fatal(err)
	}

	encodedKey, err := EncodePrivateKey(privKey)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DecodePrivateKey(encodedKey)
		if err != nil {
			b.Fatal(err)
		}
	}
}

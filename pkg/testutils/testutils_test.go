package testutils

import (
	"encoding/base64"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestHost(t *testing.T) {
	host := TestHost(t)
	require.NotNil(t, host)

	// Verify host is properly configured
	assert.NotEmpty(t, host.ID())
	assert.NotEmpty(t, host.Addrs())

	// Clean up
	host.Close()
}

func TestTestConn(t *testing.T) {
	testPeer := TestPeer(t)
	conn := &TestConn{RemotePeerID: testPeer}

	// Test RemotePeer method
	assert.Equal(t, testPeer, conn.RemotePeer())

	// Test interface methods (should not panic)
	assert.NoError(t, conn.Close())
	assert.Nil(t, conn.LocalAddr())
	assert.Nil(t, conn.RemoteAddr())
	assert.NoError(t, conn.SetDeadline(time.Now()))
	assert.NoError(t, conn.SetReadDeadline(time.Now()))
	assert.NoError(t, conn.SetWriteDeadline(time.Now()))

	// Test Read/Write methods
	buf := make([]byte, 10)
	n, err := conn.Read(buf)
	assert.Equal(t, 0, n)
	assert.NoError(t, err)

	n, err = conn.Write([]byte("test"))
	assert.Equal(t, 0, n)
	assert.NoError(t, err)
}

func TestNewTestStream(t *testing.T) {
	stream := NewTestStream()
	require.NotNil(t, stream)

	// Verify initial state
	assert.False(t, stream.closed)
	assert.Equal(t, 0, stream.readIndex)
	assert.NotNil(t, stream.readData)
	assert.NotNil(t, stream.writeData)
	assert.Nil(t, stream.conn)
	assert.Nil(t, stream.readError)
	assert.Nil(t, stream.writeError)
}

func TestTestStreamRead(t *testing.T) {
	stream := NewTestStream()
	testData := []byte("Hello, World!")
	stream.SetReadData(testData)

	// Test successful read
	buf := make([]byte, 20)
	n, err := stream.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, buf[:n])

	// Test EOF
	n, err = stream.Read(buf)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, 0, n)
}

func TestTestStreamReadWithError(t *testing.T) {
	stream := NewTestStream()
	expectedErr := assert.AnError
	stream.SetReadError(expectedErr)

	buf := make([]byte, 10)
	n, err := stream.Read(buf)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 0, n)
}

func TestTestStreamWrite(t *testing.T) {
	stream := NewTestStream()
	testData := []byte("Hello, World!")

	// Test successful write
	n, err := stream.Write(testData)
	assert.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, testData, stream.GetWriteData())

	// Test multiple writes
	additionalData := []byte("Additional data")
	n, err = stream.Write(additionalData)
	assert.NoError(t, err)
	assert.Equal(t, len(additionalData), n)

	expectedData := append(testData, additionalData...)
	assert.Equal(t, expectedData, stream.GetWriteData())
}

func TestTestStreamWriteWhenClosed(t *testing.T) {
	stream := NewTestStream()
	stream.Close()

	n, err := stream.Write([]byte("test"))
	assert.Error(t, err)
	assert.Equal(t, 0, n)
	assert.Contains(t, err.Error(), "stream closed")
}

func TestTestStreamWriteWithError(t *testing.T) {
	stream := NewTestStream()
	expectedErr := assert.AnError
	stream.SetWriteError(expectedErr)

	n, err := stream.Write([]byte("test"))
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 0, n)
}

func TestTestStreamClose(t *testing.T) {
	stream := NewTestStream()
	assert.False(t, stream.closed)

	err := stream.Close()
	assert.NoError(t, err)
	assert.True(t, stream.closed)
}

func TestTestStreamReset(t *testing.T) {
	stream := NewTestStream()
	assert.False(t, stream.closed)

	err := stream.Reset()
	assert.NoError(t, err)
	assert.True(t, stream.closed)
}

func TestTestStreamDeadlineMethods(t *testing.T) {
	stream := NewTestStream()

	// These methods should not panic and return nil
	assert.NoError(t, stream.SetReadDeadline(time.Now()))
	assert.NoError(t, stream.SetWriteDeadline(time.Now()))
	assert.NoError(t, stream.SetDeadline(time.Now()))
}

func TestTestStreamConnMethods(t *testing.T) {
	stream := NewTestStream()

	// Initially should be nil
	assert.Nil(t, stream.Conn())

	// Set a test connection
	testConn := &TestConn{RemotePeerID: TestPeer(t)}
	stream.SetConn(testConn)
	assert.Equal(t, testConn, stream.Conn())
}

func TestTestStreamDataMethods(t *testing.T) {
	stream := NewTestStream()
	testData := []byte("test data")

	// Test SetReadData
	stream.SetReadData(testData)
	assert.Equal(t, testData, stream.readData)

	// Test GetWriteData
	stream.Write(testData)
	assert.Equal(t, testData, stream.GetWriteData())
}

func TestTestStreamErrorMethods(t *testing.T) {
	stream := NewTestStream()
	expectedErr := assert.AnError

	// Test SetReadError
	stream.SetReadError(expectedErr)
	assert.Equal(t, expectedErr, stream.readError)

	// Test SetWriteError
	stream.SetWriteError(expectedErr)
	assert.Equal(t, expectedErr, stream.writeError)
}

func TestTestStreamResetReadIndex(t *testing.T) {
	stream := NewTestStream()
	testData := []byte("Hello, World!")
	stream.SetReadData(testData)

	// Read some data
	buf := make([]byte, 5)
	stream.Read(buf)
	assert.Greater(t, stream.readIndex, 0)

	// Reset read index
	stream.ResetReadIndex()
	assert.Equal(t, 0, stream.readIndex)

	// Should be able to read from beginning again
	n, err := stream.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
}

func TestTestPolicy(t *testing.T) {
	policy := &TestPolicy{
		Enable:    true,
		WhiteList: []string{"peer1", "peer2"},
	}

	// Test AuthorizeStream
	called := false
	next := func(s network.Stream) {
		called = true
	}

	handler := policy.AuthorizeStream(next)
	require.NotNil(t, handler)

	// Call the handler
	handler(nil)
	assert.True(t, called)
}

func TestTestPeer(t *testing.T) {
	peerID := TestPeer(t)
	require.NotEmpty(t, peerID)

	// Verify it's a valid peer ID
	assert.True(t, peerID.Validate() == nil)

	// Test consistency
	peerID2 := TestPeer(t)
	assert.NotEqual(t, peerID, peerID2) // Should be different each time
}

func TestTestPrivateKey(t *testing.T) {
	privKey := TestPrivateKey(t)
	require.NotNil(t, privKey)

	// Verify it's an Ed25519 key
	assert.Equal(t, int(crypto.Ed25519), int(privKey.Type()))

	// Verify we can get the public key
	pubKey := privKey.GetPublic()
	assert.NotNil(t, pubKey)
	assert.Equal(t, int(crypto.Ed25519), int(pubKey.Type()))

	// Verify we can derive peer ID
	peerID, err := peer.IDFromPrivateKey(privKey)
	assert.NoError(t, err)
	assert.NotEmpty(t, peerID.String())
}

func TestTestPrivateKeyB64(t *testing.T) {
	b64Key := TestPrivateKeyB64(t)
	require.NotEmpty(t, b64Key)

	// Verify it's valid base64
	keyBytes, err := base64.StdEncoding.DecodeString(b64Key)
	assert.NoError(t, err)
	assert.NotEmpty(t, keyBytes)

	// Verify it can be unmarshaled as a private key
	privKey, err := crypto.UnmarshalPrivateKey(keyBytes)
	assert.NoError(t, err)
	assert.NotNil(t, privKey)
}

func TestTestTempDir(t *testing.T) {
	dir := TestTempDir(t)
	require.NotEmpty(t, dir)

	// Verify directory exists
	info, err := os.Stat(dir)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify it's in the temp directory
	tempDir := os.TempDir()
	assert.Contains(t, dir, tempDir)

	// Verify it has the expected prefix
	assert.Contains(t, filepath.Base(dir), "stellar-test-")
}

func TestTestTempFile(t *testing.T) {
	content := "Hello, World! This is test content."
	filePath := TestTempFile(t, content)
	require.NotEmpty(t, filePath)

	// Verify file exists
	info, err := os.Stat(filePath)
	assert.NoError(t, err)
	assert.False(t, info.IsDir())

	// Verify content
	fileContent, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	assert.Equal(t, content, string(fileContent))

	// Verify it's in a temp directory
	tempDir := os.TempDir()
	assert.Contains(t, filePath, tempDir)
}

func TestTestFreePort(t *testing.T) {
	port := TestFreePort(t)
	assert.Greater(t, port, 0)
	assert.Less(t, port, 65536)

	// Verify the port is actually free
	addr := net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}
	listener, err := net.ListenTCP("tcp", &addr)
	assert.NoError(t, err)
	listener.Close()
}

func TestTestContext(t *testing.T) {
	ctx := TestContext(t)
	require.NotNil(t, ctx)

	// Verify it has a deadline
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.True(t, deadline.After(time.Now()))

	// Verify it's not done initially
	select {
	case <-ctx.Done():
		t.Fatal("Context should not be done initially")
	default:
		// Expected
	}
}

func TestAssertHelpers(t *testing.T) {
	// Test AssertNoError
	AssertNoError(t, nil)

	// Test AssertError
	AssertError(t, assert.AnError)

	// Test AssertEqual
	AssertEqual(t, "test", "test")
	AssertEqual(t, 42, 42)

	// Test AssertNotNil
	AssertNotNil(t, "test")
	AssertNotNil(t, 42)

	// Test AssertNil
	AssertNil(t, nil)

	// Test AssertTrue
	AssertTrue(t, true)

	// Test AssertFalse
	AssertFalse(t, false)
}

func BenchmarkTestHost(b *testing.B) {
	for i := 0; i < b.N; i++ {
		host := TestHost(&testing.T{})
		host.Close()
	}
}

func BenchmarkTestPeer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestPeer(&testing.T{})
	}
}

func BenchmarkTestPrivateKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestPrivateKey(&testing.T{})
	}
}

func BenchmarkTestTempDir(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestTempDir(&testing.T{})
	}
}

func BenchmarkTestFreePort(b *testing.B) {
	for i := 0; i < b.N; i++ {
		TestFreePort(&testing.T{})
	}
}

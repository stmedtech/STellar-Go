package testutils

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// TestHost creates a test libp2p host
func TestHost(t *testing.T) host.Host {
	t.Helper()

	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 2048, rand.Reader)
	require.NoError(t, err)

	host, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Security(noise.ID, noise.New),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
	)
	require.NoError(t, err)

	return host
}

// TestConn creates a mock network connection for testing
type TestConn struct {
	RemotePeerID peer.ID
}

func (tc *TestConn) RemotePeer() peer.ID {
	return tc.RemotePeerID
}

func (tc *TestConn) LocalPeer() peer.ID {
	return peer.ID("") // Return empty peer ID for testing
}

func (tc *TestConn) RemotePublicKey() crypto.PubKey {
	return nil // Return nil for testing
}

func (tc *TestConn) Scope() network.ConnScope {
	return nil // Return nil for testing
}

func (tc *TestConn) Stat() network.ConnStats {
	return network.ConnStats{} // Return empty stats for testing
}

// Implement network.Conn interface
func (tc *TestConn) Close() error                                      { return nil }
func (tc *TestConn) CloseWithError(network.ConnErrorCode) error        { return nil }
func (tc *TestConn) ConnState() network.ConnectionState                { return network.ConnectionState{} }
func (tc *TestConn) ID() string                                        { return "test-conn-id" }
func (tc *TestConn) NewStream(context.Context) (network.Stream, error) { return nil, nil }
func (tc *TestConn) GetStreams() []network.Stream                      { return nil }
func (tc *TestConn) IsClosed() bool                                    { return false }
func (tc *TestConn) LocalMultiaddr() multiaddr.Multiaddr               { return nil }
func (tc *TestConn) RemoteMultiaddr() multiaddr.Multiaddr              { return nil }
func (tc *TestConn) LocalAddr() net.Addr                               { return nil }
func (tc *TestConn) RemoteAddr() net.Addr                              { return nil }
func (tc *TestConn) SetDeadline(time.Time) error                       { return nil }
func (tc *TestConn) SetReadDeadline(time.Time) error                   { return nil }
func (tc *TestConn) SetWriteDeadline(time.Time) error                  { return nil }
func (tc *TestConn) Read(p []byte) (n int, err error)                  { return 0, nil }
func (tc *TestConn) Write(p []byte) (n int, err error)                 { return 0, nil }

// TestStream creates a mock network stream for testing
type TestStream struct {
	network.Stream
	readData   []byte
	writeData  []byte
	closed     bool
	conn       network.Conn
	readError  error
	writeError error
	readIndex  int
}

func NewTestStream() *TestStream {
	return &TestStream{
		readData:  make([]byte, 0),
		writeData: make([]byte, 0),
		closed:    false,
		readIndex: 0,
		conn:      nil, // Explicitly set to nil
	}
}

func (ts *TestStream) Read(p []byte) (n int, err error) {
	if ts.readError != nil {
		return 0, ts.readError
	}

	if ts.readIndex >= len(ts.readData) {
		return 0, io.EOF
	}

	n = copy(p, ts.readData[ts.readIndex:])
	ts.readIndex += n
	return n, nil
}

func (ts *TestStream) Write(p []byte) (n int, err error) {
	if ts.writeError != nil {
		return 0, ts.writeError
	}
	if ts.closed {
		return 0, fmt.Errorf("stream closed")
	}
	ts.writeData = append(ts.writeData, p...)
	return len(p), nil
}

func (ts *TestStream) Close() error {
	ts.closed = true
	return nil
}

func (ts *TestStream) Reset() error {
	ts.closed = true
	return nil
}

func (ts *TestStream) SetReadDeadline(time.Time) error  { return nil }
func (ts *TestStream) SetWriteDeadline(time.Time) error { return nil }
func (ts *TestStream) SetDeadline(time.Time) error      { return nil }

func (ts *TestStream) Conn() network.Conn {
	return ts.conn
}

func (ts *TestStream) SetConn(conn network.Conn) {
	ts.conn = conn
}

func (ts *TestStream) SetReadData(data []byte) {
	ts.readData = data
}

func (ts *TestStream) GetWriteData() []byte {
	return ts.writeData
}

func (ts *TestStream) SetReadError(err error) {
	ts.readError = err
}

func (ts *TestStream) SetWriteError(err error) {
	ts.writeError = err
}

func (ts *TestStream) ResetReadIndex() {
	ts.readIndex = 0
}

// TestNode creates a mock node for testing
type TestNode struct {
	Host   host.Host
	Policy *TestPolicy
}

// TestPolicy creates a mock policy for testing
type TestPolicy struct {
	Enable    bool
	WhiteList []string
}

func (tp *TestPolicy) AuthorizeStream(next func(network.Stream)) func(network.Stream) {
	return func(s network.Stream) {
		if !tp.Enable {
			next(s)
			return
		}
		// Simple authorization logic for testing
		next(s)
	}
}

// TestPeer creates a test peer ID
func TestPeer(t *testing.T) peer.ID {
	t.Helper()

	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 2048, rand.Reader)
	require.NoError(t, err)

	peerID, err := peer.IDFromPrivateKey(priv)
	require.NoError(t, err)

	return peerID
}

// TestPrivateKey generates a test private key
func TestPrivateKey(t *testing.T) crypto.PrivKey {
	t.Helper()

	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 2048, rand.Reader)
	require.NoError(t, err)

	return priv
}

// TestPrivateKeyB64 generates a base64 encoded test private key
func TestPrivateKeyB64(t *testing.T) string {
	t.Helper()

	priv := TestPrivateKey(t)
	privBytes, err := crypto.MarshalPrivateKey(priv)
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString(privBytes)
}

// TestTempDir creates a temporary directory for testing
func TestTempDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "stellar-test-*")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	return dir
}

// TestTempFile creates a temporary file with content
func TestTempFile(t *testing.T, content string) string {
	t.Helper()

	dir := TestTempDir(t)
	filePath := filepath.Join(dir, "test.txt")

	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	return filePath
}

// TestFreePort finds a free port for testing
func TestFreePort(t *testing.T) int {
	t.Helper()

	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)

	l, err := net.ListenTCP("tcp", addr)
	require.NoError(t, err)
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port
}

// TestContext creates a test context with timeout
func TestContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	return ctx
}

// AssertNoError is a helper to assert no error occurred
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	require.NoError(t, err)
}

// AssertError is a helper to assert an error occurred
func AssertError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
}

// AssertEqual is a helper to assert equality
func AssertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	require.Equal(t, expected, actual)
}

// AssertNotNil is a helper to assert not nil
func AssertNotNil(t *testing.T, obj interface{}) {
	t.Helper()
	require.NotNil(t, obj)
}

// AssertNil is a helper to assert nil
func AssertNil(t *testing.T, obj interface{}) {
	t.Helper()
	require.Nil(t, obj)
}

// AssertTrue is a helper to assert true
func AssertTrue(t *testing.T, condition bool) {
	t.Helper()
	require.True(t, condition)
}

// AssertFalse is a helper to assert false
func AssertFalse(t *testing.T, condition bool) {
	t.Helper()
	require.False(t, condition)
}

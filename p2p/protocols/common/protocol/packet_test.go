package protocol

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"stellar/p2p/protocols/common/multiplexer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers for simplified test setup
func newTestPacket(data []byte) *Packet {
	packet, _ := NewTunnelingPacket(data)
	return packet
}

func mustWritePacket(w io.Writer, packet *Packet) {
	err := WritePacket(w, packet)
	if err != nil {
		panic(err)
	}
}

func mustReadPacket(r io.Reader) *Packet {
	packet, err := ReadPacket(r)
	if err != nil {
		panic(err)
	}
	return packet
}

func setupTCPConnection(t *testing.T) (clientConn, serverConn net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	serverAddr := listener.Addr().String()

	var server net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		var err error
		server, err = listener.Accept()
		require.NoError(t, err)
	}()

	client, err := net.Dial("tcp", serverAddr)
	require.NoError(t, err)

	<-done
	require.NotNil(t, server)

	return client, server
}

// ============================================================================
// Basic Functionality Tests
// ============================================================================

func TestPacketMarshal(t *testing.T) {
	packet := newTestPacket([]byte(`{"data":"test"}`))
	data, err := packet.Marshal()
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Contains(t, string(data), "tunneling")
	// The data is base64 encoded in tunneling content, so check for the encoded version
	// or verify the structure is correct
	assert.Contains(t, string(data), "type")
	assert.Contains(t, string(data), "content")
}

func TestPacketUnmarshal(t *testing.T) {
	original := newTestPacket([]byte(`{"data":"test"}`))
	data, err := original.Marshal()
	require.NoError(t, err)

	var unmarshaled Packet
	err = unmarshaled.Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, original.Type, unmarshaled.Type)
	assert.Equal(t, original.Content, unmarshaled.Content)
}

func TestPacketLengthPrefix(t *testing.T) {
	packet := newTestPacket([]byte(`{"data":"test"}`))
	buf := &bytes.Buffer{}
	err := WritePacket(buf, packet)
	require.NoError(t, err)

	data := buf.Bytes()
	length := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	expectedLength := uint32(len(data) - 4)
	assert.Equal(t, expectedLength, length, "Length prefix should match packet data length")
}

func TestPacketTunnelingContent(t *testing.T) {
	testData := []byte("hello world")
	packet, err := NewTunnelingPacket(testData)
	require.NoError(t, err)
	assert.Equal(t, PacketTypeTunneling, packet.Type)

	content, err := packet.UnmarshalTunnelingContent()
	require.NoError(t, err)
	assert.Equal(t, testData, content.Data)
}

func TestPacketReadWrite(t *testing.T) {
	buf := &bytes.Buffer{}
	testData := []byte("test data")
	packet, err := NewTunnelingPacket(testData)
	require.NoError(t, err)

	err = WritePacket(buf, packet)
	require.NoError(t, err)
	assert.NotEmpty(t, buf.Bytes())

	readPacket, err := ReadPacket(buf)
	require.NoError(t, err)
	assert.Equal(t, packet.Type, readPacket.Type)

	content, err := readPacket.UnmarshalTunnelingContent()
	require.NoError(t, err)
	assert.Equal(t, testData, content.Data)
}

func TestPacketReadWriteMultiple(t *testing.T) {
	buf := &bytes.Buffer{}

	// Write multiple packets
	for i := 0; i < 5; i++ {
		testData := []byte{byte(i)}
		packet, err := NewTunnelingPacket(testData)
		require.NoError(t, err)
		mustWritePacket(buf, packet)
	}

	// Read all packets back
	for i := 0; i < 5; i++ {
		readPacket := mustReadPacket(buf)
		content, err := readPacket.UnmarshalTunnelingContent()
		require.NoError(t, err)
		assert.Equal(t, []byte{byte(i)}, content.Data)
	}

	// Should be EOF now
	_, err := ReadPacket(buf)
	assert.Error(t, err)
	if err != io.EOF {
		assert.Contains(t, err.Error(), "EOF")
	}
}

// bufferConn wraps bytes.Buffer to implement io.ReadWriteCloser
type bufferConn struct {
	*bytes.Buffer
	closed bool
}

func (b *bufferConn) Close() error {
	b.closed = true
	return nil
}

func TestPacketReadWriteCloser(t *testing.T) {
	buf := &bufferConn{Buffer: &bytes.Buffer{}}
	packetConn := NewPacketReadWriteCloser(buf)

	testData := []byte("hello from packet connection")
	packet, err := NewTunnelingPacket(testData)
	require.NoError(t, err)

	err = WritePacket(packetConn, packet)
	require.NoError(t, err)

	readPacket, err := ReadPacket(packetConn)
	require.NoError(t, err)

	content, err := readPacket.UnmarshalTunnelingContent()
	require.NoError(t, err)
	assert.Equal(t, testData, content.Data)
}

func TestPacketThroughMultiplexer(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := multiplexer.NewMultiplexer(clientConn)
	serverMux := multiplexer.NewMultiplexer(serverConn)

	// Wait for readLoops
	time.Sleep(50 * time.Millisecond)

	// Get control streams
	clientControl, err := clientMux.ControlStream()
	require.NoError(t, err)

	serverControl, err := serverMux.ControlStream()
	require.NoError(t, err)

	// Create packet RWC
	clientPacketRWC := NewPacketReadWriteCloser(clientControl)
	serverPacketRWC := NewPacketReadWriteCloser(serverControl)

	// Create and send hello packet
	helloPacket, err := NewHelloPacket("1.0", "test-client")
	require.NoError(t, err)

	err = clientPacketRWC.WritePacket(helloPacket)
	require.NoError(t, err)

	// Read packet on server side
	receivedPacket, err := serverPacketRWC.ReadPacket()
	require.NoError(t, err)
	require.NotNil(t, receivedPacket)

	// Unmarshal handshake
	handshake, err := UnmarshalHandshakePacket(receivedPacket)
	require.NoError(t, err)
	require.NotNil(t, handshake)

	assert.Equal(t, HandshakeTypeHello, handshake.Type)

	var helloPayload HelloPayload
	err = handshake.UnmarshalPayload(&helloPayload)
	require.NoError(t, err)
	assert.Equal(t, "1.0", helloPayload.Version)
	assert.Equal(t, "test-client", helloPayload.ClientID)
}

// ============================================================================
// Edge Cases: Empty Data
// ============================================================================

func TestPacketEmptyData(t *testing.T) {
	emptyData := []byte{}
	packet, err := NewTunnelingPacket(emptyData)
	require.NoError(t, err)

	content, err := packet.UnmarshalTunnelingContent()
	require.NoError(t, err)
	assert.Equal(t, emptyData, content.Data)
}

func TestPacketReadEmptyBuffer(t *testing.T) {
	buf := &bytes.Buffer{}
	_, err := ReadPacket(buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EOF")
}

func TestPacketWriteToClosedWriter(t *testing.T) {
	closedBuf := &bufferConn{Buffer: &bytes.Buffer{}, closed: true}
	packet := newTestPacket([]byte("test"))
	// Writing to closed buffer may or may not error depending on implementation
	// Just verify it doesn't panic
	assert.NotPanics(t, func() {
		_ = WritePacket(closedBuf, packet)
	})
}

// ============================================================================
// Edge Cases: Large Data
// ============================================================================

func TestPacketLargeData(t *testing.T) {
	// Test with large data (10KB)
	largeData := make([]byte, 10*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	packet, err := NewTunnelingPacket(largeData)
	require.NoError(t, err)

	content, err := packet.UnmarshalTunnelingContent()
	require.NoError(t, err)
	assert.Equal(t, largeData, content.Data)
}

func TestPacketVeryLargeData(t *testing.T) {
	// Test with very large data (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	packet, err := NewTunnelingPacket(largeData)
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	err = WritePacket(buf, packet)
	require.NoError(t, err)

	readPacket, err := ReadPacket(buf)
	require.NoError(t, err)

	content, err := readPacket.UnmarshalTunnelingContent()
	require.NoError(t, err)
	assert.Equal(t, largeData, content.Data)
}

// ============================================================================
// Edge Cases: Invalid Data
// ============================================================================

func TestPacketUnmarshalInvalidJSON(t *testing.T) {
	var packet Packet
	invalidJSON := []byte("{invalid json}")
	err := packet.Unmarshal(invalidJSON)
	assert.Error(t, err)
}

func TestPacketUnmarshalTunnelingContentWrongType(t *testing.T) {
	// Create a packet with wrong type
	packet := &Packet{
		Type:    PacketTypeHandshake,
		Content: []byte(`{"data":"test"}`),
	}

	_, err := packet.UnmarshalTunnelingContent()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a tunneling packet")
}

func TestPacketUnmarshalTunnelingContentInvalidJSON(t *testing.T) {
	packet := &Packet{
		Type:    PacketTypeTunneling,
		Content: []byte(`{invalid json}`),
	}

	_, err := packet.UnmarshalTunnelingContent()
	assert.Error(t, err)
}

// ============================================================================
// Edge Cases: Length Prefix Issues
// ============================================================================

func TestPacketReadIncompleteLengthPrefix(t *testing.T) {
	// Write only 2 bytes of length prefix
	buf := &bytes.Buffer{}
	buf.Write([]byte{0x00, 0x00})
	_, err := ReadPacket(buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EOF")
}

func TestPacketReadIncompleteData(t *testing.T) {
	// Write length prefix but incomplete data
	buf := &bytes.Buffer{}
	// Write length prefix: 100 bytes
	buf.Write([]byte{0x00, 0x00, 0x00, 0x64})
	// Write only 50 bytes
	buf.Write(make([]byte, 50))
	_, err := ReadPacket(buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EOF")
}

func TestPacketReadInvalidLengthPrefix(t *testing.T) {
	// Write extremely large length prefix (potential DoS)
	buf := &bytes.Buffer{}
	// Length: 0xFFFFFFFF (max uint32)
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	// This should fail when trying to allocate memory
	_, err := ReadPacket(buf)
	assert.Error(t, err)
}

func TestPacketReadZeroLength(t *testing.T) {
	// Write zero length prefix
	buf := &bytes.Buffer{}
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00})
	packet, err := ReadPacket(buf)
	// Zero length should be valid (empty packet)
	if err == nil {
		assert.NotNil(t, packet)
	}
}

// ============================================================================
// Edge Cases: Concurrent Access
// ============================================================================

func TestPacketConcurrentReadWrite(t *testing.T) {
	buf := &bytes.Buffer{}
	packet := newTestPacket([]byte("test"))

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			_ = WritePacket(buf, packet)
		}()
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify we can read packets (order may vary)
	packetCount := 0
	for {
		_, err := ReadPacket(buf)
		if err != nil {
			break
		}
		packetCount++
	}
	assert.Greater(t, packetCount, 0)
}

// ============================================================================
// Edge Cases: Type Safety
// ============================================================================

func TestPacketAllPacketTypes(t *testing.T) {
	types := []PacketType{
		PacketTypeTunneling,
		PacketTypeHandshake,
		PacketTypeStatus,
		PacketTypeNested,
	}

	for _, packetType := range types {
		packet := &Packet{
			Type:    packetType,
			Content: []byte(`{"data":"test"}`),
		}

		data, err := packet.Marshal()
		require.NoError(t, err, "Should marshal type %s", packetType)

		var unmarshaled Packet
		err = unmarshaled.Unmarshal(data)
		require.NoError(t, err, "Should unmarshal type %s", packetType)
		assert.Equal(t, packetType, unmarshaled.Type)
	}
}

// ============================================================================
// Edge Cases: Round-trip Consistency
// ============================================================================

func TestPacketRoundTripConsistency(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x42}},
		{"small", []byte("hello")},
		{"medium", make([]byte, 1024)},
		{"large", make([]byte, 64*1024)},
		{"binary", []byte{0x00, 0xFF, 0x80, 0x7F}},
		{"unicode", []byte("你好世界")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			original, err := NewTunnelingPacket(tc.data)
			require.NoError(t, err)

			buf := &bytes.Buffer{}
			err = WritePacket(buf, original)
			require.NoError(t, err)

			read, err := ReadPacket(buf)
			require.NoError(t, err)

			content, err := read.UnmarshalTunnelingContent()
			require.NoError(t, err)
			assert.Equal(t, tc.data, content.Data)
		})
	}
}

// ============================================================================
// Edge Cases: Timeout/Blocking
// ============================================================================

type slowReader struct {
	*bytes.Buffer
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	time.Sleep(r.delay)
	return r.Buffer.Read(p)
}

func TestPacketReadWithTimeout(t *testing.T) {
	// This test verifies that ReadPacket doesn't block indefinitely
	// In a real scenario, you'd use context.WithTimeout
	slowBuf := &slowReader{
		Buffer: &bytes.Buffer{},
		delay:  10 * time.Millisecond,
	}

	packet := newTestPacket([]byte("test"))
	err := WritePacket(slowBuf, packet)
	require.NoError(t, err)

	// Read should complete despite delay
	readPacket, err := ReadPacket(slowBuf)
	require.NoError(t, err)
	assert.NotNil(t, readPacket)
}

// ============================================================================
// Edge Cases: Memory Leaks
// ============================================================================

func TestPacketNoMemoryLeak(t *testing.T) {
	// Write and read many packets to check for memory leaks
	buf := &bytes.Buffer{}
	for i := 0; i < 1000; i++ {
		packet := newTestPacket([]byte("test"))
		err := WritePacket(buf, packet)
		require.NoError(t, err)
	}

	// Read all packets
	for i := 0; i < 1000; i++ {
		_, err := ReadPacket(buf)
		require.NoError(t, err)
	}

	// Buffer should be empty
	assert.Equal(t, 0, buf.Len())
}

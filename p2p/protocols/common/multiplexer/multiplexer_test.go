package multiplexer

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers for simplified test setup
func setupTCPConnection(t *testing.T) (clientConn, serverConn net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	serverAddr := listener.Addr().String()

	var server net.Conn
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		server, err = listener.Accept()
		require.NoError(t, err)
	}()

	client, err := net.Dial("tcp", serverAddr)
	require.NoError(t, err)

	wg.Wait()
	require.NotNil(t, server)

	return client, server
}

func waitForStream(mux *Multiplexer, streamID uint32, timeout time.Duration) (*Stream, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stream, err := mux.GetStream(streamID)
		if err == nil && stream != nil {
			return stream, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, ErrStreamNotFound
}

// ============================================================================
// Basic Functionality Tests
// ============================================================================

func TestMultiplexerStreamCreation(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	testData := []byte("test data")
	_, err = clientStream.Write(testData)
	require.NoError(t, err)

	serverStream, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, serverStream)

	buffer := make([]byte, 100)
	n, err := serverStream.Read(buffer)
	require.NoError(t, err)
	assert.Equal(t, "test data", string(buffer[:n]))
}

func TestMultiplexerStreamReadWrite(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	testData := []byte("hello from client")
	_, err = clientStream.Write(testData)
	require.NoError(t, err)

	serverStream, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)

	buffer := make([]byte, 100)
	n, err := serverStream.Read(buffer)
	require.NoError(t, err)
	assert.Equal(t, "hello from client", string(buffer[:n]))

	responseData := []byte("hello from server")
	_, err = serverStream.Write(responseData)
	require.NoError(t, err)

	responseBuffer := make([]byte, 100)
	n, err = clientStream.Read(responseBuffer)
	require.NoError(t, err)
	assert.Equal(t, "hello from server", string(responseBuffer[:n]))
}

func TestMultiplexerBasic(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)

	// Wait for readLoops to start
	time.Sleep(50 * time.Millisecond)

	// Get control streams
	clientControl, err := clientMux.ControlStream()
	require.NoError(t, err)

	serverControl, err := serverMux.ControlStream()
	require.NoError(t, err)

	// Write from client
	testData := []byte("hello from client")
	_, err = clientControl.Write(testData)
	require.NoError(t, err)

	// Read from server
	buffer := make([]byte, 100)
	n, err := serverControl.Read(buffer)
	require.NoError(t, err)
	assert.Equal(t, "hello from client", string(buffer[:n]))

	// Write from server
	responseData := []byte("hello from server")
	_, err = serverControl.Write(responseData)
	require.NoError(t, err)

	// Read from client
	responseBuffer := make([]byte, 100)
	n, err = clientControl.Read(responseBuffer)
	require.NoError(t, err)
	assert.Equal(t, "hello from server", string(responseBuffer[:n]))
}

func TestMultiplexerMultipleStreams(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	numStreams := 5
	clientStreams := make([]*Stream, numStreams)
	serverStreams := make([]*Stream, numStreams)

	for i := 0; i < numStreams; i++ {
		streamID := uint32(i + 1)
		clientStream, err := clientMux.OpenStreamWithID(streamID)
		require.NoError(t, err)
		clientStreams[i] = clientStream

		testData := []byte{byte(i)}
		_, err = clientStream.Write(testData)
		require.NoError(t, err)

		serverStream, err := waitForStream(serverMux, streamID, 5*time.Second)
		require.NoError(t, err)
		serverStreams[i] = serverStream
	}

	for i := 0; i < numStreams; i++ {
		buffer := make([]byte, 10)
		n, err := serverStreams[i].Read(buffer)
		require.NoError(t, err)
		assert.Equal(t, []byte{byte(i)}, buffer[:n])
	}
}

func TestMultiplexerStreamAutoCreation(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(10)
	require.NoError(t, err)

	testData := []byte("auto-create test")
	_, err = clientStream.Write(testData)
	require.NoError(t, err)

	serverStream, err := waitForStream(serverMux, 10, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, serverStream)

	buffer := make([]byte, 100)
	n, err := serverStream.Read(buffer)
	require.NoError(t, err)
	assert.Equal(t, "auto-create test", string(buffer[:n]))
}

func TestMultiplexerStreamClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	_, err = clientStream.Write([]byte("test"))
	require.NoError(t, err)

	serverStream, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, serverStream)

	err = clientStream.Close()
	require.NoError(t, err)
	assert.True(t, clientStream.IsClosed())
	assert.NotNil(t, serverStream) // Verify server stream exists
}

func TestMultiplexerConcurrentStreams(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	numStreams := 10
	var wg sync.WaitGroup
	wg.Add(numStreams * 2)

	for i := 0; i < numStreams; i++ {
		streamID := uint32(i + 100)
		go func(id uint32) {
			defer wg.Done()
			clientStream, err := clientMux.OpenStreamWithID(id)
			require.NoError(t, err)

			testData := []byte{byte(id)}
			_, err = clientStream.Write(testData)
			require.NoError(t, err)
		}(streamID)

		go func(id uint32) {
			defer wg.Done()
			serverStream, err := waitForStream(serverMux, id, 5*time.Second)
			require.NoError(t, err)

			buffer := make([]byte, 10)
			n, err := serverStream.Read(buffer)
			require.NoError(t, err)
			assert.Equal(t, []byte{byte(id)}, buffer[:n])
		}(streamID)
	}

	wg.Wait()
}

// ============================================================================
// Edge Cases: Stream ID Management
// ============================================================================

func TestMultiplexerDuplicateStreamID(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	stream1, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)
	require.NotNil(t, stream1)

	// Try to open stream with same ID
	stream2, err := clientMux.OpenStreamWithID(1)
	assert.Error(t, err)
	assert.Nil(t, stream2)
}

func TestMultiplexerAutoAssignStreamID(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	// Open multiple streams with auto-assign
	streams := make([]*Stream, 5)
	for i := 0; i < 5; i++ {
		stream, err := clientMux.OpenStream()
		require.NoError(t, err)
		require.NotNil(t, stream)
		streams[i] = stream
	}

	// Verify all have different IDs
	ids := make(map[uint32]bool)
	for _, stream := range streams {
		assert.False(t, ids[stream.ID], "Stream ID %d should be unique", stream.ID)
		ids[stream.ID] = true
	}
}

func TestMultiplexerStreamIDZero(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	stream, err := clientMux.ControlStream()
	require.NoError(t, err)
	require.NotNil(t, stream)
	assert.Equal(t, uint32(0), stream.ID)

	_, err = clientMux.OpenStreamWithID(0)
	assert.Equal(t, ErrInvalidStreamID, err)
}

// ============================================================================
// Edge Cases: Stream Operations After Close
// ============================================================================

func TestMultiplexerWriteAfterStreamClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	stream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	err = stream.Close()
	require.NoError(t, err)

	// Write after close should fail
	_, err = stream.Write([]byte("test"))
	assert.Error(t, err)
}

func TestMultiplexerReadAfterStreamClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	_ = NewMultiplexer(serverConn) // Server mux needed for connection
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	err = clientStream.Close()
	require.NoError(t, err)

	// Read after close should return EOF
	buffer := make([]byte, 10)
	n, err := clientStream.Read(buffer)
	assert.Equal(t, 0, n)
	assert.Error(t, err)
}

func TestMultiplexerCloseAfterMultiplexerClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	stream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	err = clientMux.Close()
	require.NoError(t, err)

	// Closing stream after multiplexer close should not panic
	assert.NotPanics(t, func() {
		_ = stream.Close()
	})
}

// ============================================================================
// Edge Cases: Large Data
// ============================================================================

func TestMultiplexerLargeData(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	// Write 1MB of data
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	_, err = clientStream.Write(largeData)
	require.NoError(t, err)

	serverStream, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)

	// Read in chunks
	readBuffer := make([]byte, 64*1024)
	totalRead := 0
	for totalRead < len(largeData) {
		n, err := serverStream.Read(readBuffer)
		if err != nil {
			break
		}
		totalRead += n
	}

	assert.Equal(t, len(largeData), totalRead)
}

func TestMultiplexerEmptyData(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	// Write empty data
	_, err = clientStream.Write([]byte{})
	require.NoError(t, err)

	serverStream, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)

	// Empty write should be delivered
	buffer := make([]byte, 10)
	n, err := serverStream.Read(buffer)
	// Empty read may return 0 or EOF
	assert.True(t, n == 0 || err != nil)
}

// ============================================================================
// Edge Cases: Connection Errors
// ============================================================================

func TestMultiplexerConnectionClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	// Write some data first
	_, err = clientStream.Write([]byte("test"))
	require.NoError(t, err)

	// Get server stream
	serverStream, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)

	// Read the data that was written before closing
	buffer := make([]byte, 10)
	n, err := serverStream.Read(buffer)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "test", string(buffer[:n]))

	// Close underlying connection
	clientConn.Close()

	// Write should fail
	_, err = clientStream.Write([]byte("test"))
	assert.Error(t, err)

	// Wait for readLoop to detect closure
	time.Sleep(100 * time.Millisecond)

	// Read should now fail (connection closed, no more data)
	_, err = serverStream.Read(buffer)
	assert.Error(t, err)
}

// ============================================================================
// Edge Cases: Deadlock Prevention
// ============================================================================

func TestMultiplexerConcurrentWritesNoDeadlock(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	_ = NewMultiplexer(serverConn) // Server mux needed for connection
	time.Sleep(100 * time.Millisecond)

	// Create multiple streams
	numStreams := 10
	streams := make([]*Stream, numStreams)
	for i := 0; i < numStreams; i++ {
		stream, err := clientMux.OpenStreamWithID(uint32(i + 1))
		require.NoError(t, err)
		streams[i] = stream
	}

	// Concurrent writes from all streams
	done := make(chan bool, numStreams)
	for i, stream := range streams {
		go func(s *Stream, id int) {
			defer func() { done <- true }()
			data := []byte{byte(id)}
			for j := 0; j < 100; j++ {
				_, err := s.Write(data)
				if err != nil {
					return
				}
			}
		}(stream, i)
	}

	// Wait with timeout to detect deadlocks
	timeout := time.After(5 * time.Second)
	for i := 0; i < numStreams; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Deadlock detected: concurrent writes timed out")
		}
	}
}

func TestMultiplexerConcurrentReadWriteNoDeadlock(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	// Write initial data to trigger server-side stream creation
	_, err = clientStream.Write([]byte{0})
	require.NoError(t, err)

	// Wait for server stream to be created
	serverStream, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)

	// Read the initial byte
	buffer := make([]byte, 1)
	_, err = serverStream.Read(buffer)
	require.NoError(t, err)

	// Concurrent read and write
	done := make(chan bool, 2)

	go func() {
		defer func() { done <- true }()
		for i := 1; i < 100; i++ {
			data := []byte{byte(i)}
			_, err := clientStream.Write(data)
			if err != nil {
				return
			}
		}
	}()

	go func() {
		defer func() { done <- true }()
		buffer := make([]byte, 1)
		for i := 0; i < 99; i++ { // Read 99 more (already read 1)
			_, err := serverStream.Read(buffer)
			if err != nil {
				return
			}
		}
	}()

	// Wait with timeout
	timeout := time.After(5 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Deadlock detected: concurrent read/write timed out")
		}
	}
}

func TestMultiplexerConcurrentStreamCreationNoDeadlock(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	// Concurrent stream creation
	numGoroutines := 20
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			streamID := uint32(id + 1)
			stream, err := clientMux.OpenStreamWithID(streamID)
			if err == nil {
				_, _ = stream.Write([]byte{byte(id)})
			}
		}(i)
	}

	// Wait with timeout
	timeout := time.After(5 * time.Second)
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Deadlock detected: concurrent stream creation timed out")
		}
	}
}

// ============================================================================
// Edge Cases: Race Conditions
// ============================================================================

func TestMultiplexerRaceConditionStreamClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	stream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	// Concurrent close operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			_ = stream.Close()
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Stream should be closed
	assert.True(t, stream.IsClosed())
}

func TestMultiplexerRaceConditionGetStream(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	stream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	_, err = stream.Write([]byte("test"))
	require.NoError(t, err)

	// Concurrent GetStream calls
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			_, _ = serverMux.GetStream(1)
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// ============================================================================
// Edge Cases: Multiplexer Close
// ============================================================================

func TestMultiplexerCloseAllStreams(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	// Create multiple streams
	streams := make([]*Stream, 5)
	for i := 0; i < 5; i++ {
		stream, err := clientMux.OpenStreamWithID(uint32(i + 1))
		require.NoError(t, err)
		streams[i] = stream
	}

	// Close multiplexer
	err := clientMux.Close()
	require.NoError(t, err)

	// All streams should be closed
	for _, stream := range streams {
		assert.True(t, stream.IsClosed())
	}
}

func TestMultiplexerCloseTwice(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	err := clientMux.Close()
	require.NoError(t, err)

	// Close again should not error
	err = clientMux.Close()
	assert.NoError(t, err)
}

func TestMultiplexerOperationsAfterClose(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	time.Sleep(100 * time.Millisecond)

	err := clientMux.Close()
	require.NoError(t, err)

	// Operations after close should fail
	_, err = clientMux.OpenStreamWithID(1)
	assert.Error(t, err)

	_, err = clientMux.GetStream(1)
	assert.Error(t, err)
}

// ============================================================================
// Edge Cases: Stream Buffer Overflow
// ============================================================================

func TestMultiplexerSlowReader(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	clientStream, err := clientMux.OpenStreamWithID(1)
	require.NoError(t, err)

	// Write a small packet first to trigger server-side stream creation
	_, err = clientStream.Write([]byte{0})
	require.NoError(t, err)

	// Now wait for server stream to be created
	serverStream, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)

	// Read the first byte
	buffer := make([]byte, 1)
	_, err = serverStream.Read(buffer)
	require.NoError(t, err)

	// Write many small packets quickly
	done := make(chan bool)
	go func() {
		defer func() { done <- true }()
		for i := 1; i < 1000; i++ {
			data := []byte{byte(i)}
			_, err := clientStream.Write(data)
			if err != nil {
				return
			}
		}
	}()

	// Read slowly (we already read 1 byte, so read 999 more)
	timeout := time.After(10 * time.Second)
	readCount := 1 // Already read the first byte
	for readCount < 1000 {
		select {
		case <-timeout:
			t.Fatal("Slow reader test timed out")
		default:
			buffer := make([]byte, 1)
			n, err := serverStream.Read(buffer)
			if err != nil {
				break
			}
			if n > 0 {
				readCount++
				time.Sleep(1 * time.Millisecond) // Slow down reading
			}
		}
	}

	<-done
	assert.GreaterOrEqual(t, readCount, 1000)
}

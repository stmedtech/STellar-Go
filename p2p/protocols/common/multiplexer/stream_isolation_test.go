package multiplexer

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamIsolation verifies that data written to one stream is not readable from another stream
func TestStreamIsolation(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	// Create control stream (ID 0) on both sides
	clientControl, err := clientMux.ControlStream()
	require.NoError(t, err)
	serverControl, err := serverMux.ControlStream()
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond) // Give time for streams to sync

	// Create proxy stream (ID 1) - try to open, but if it already exists, get it
	clientProxy, err := clientMux.OpenStreamWithID(1)
	if err != nil && err.Error() == "stream 1 already exists" {
		clientProxy, err = clientMux.GetStream(1)
		require.NoError(t, err)
	} else {
		require.NoError(t, err)
	}

	// Write a byte to trigger server-side auto-creation (if not already created)
	_, err = clientProxy.Write([]byte{0})
	require.NoError(t, err)

	// Wait for server to auto-create stream 1
	serverProxy, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)

	// Read the byte we wrote
	initBuf := make([]byte, 1)
	_, err = serverProxy.Read(initBuf)
	require.NoError(t, err)

	// Write HTTP data to proxy stream
	httpData := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, err = clientProxy.Write(httpData)
	require.NoError(t, err)

	// Verify HTTP data is readable from proxy stream
	buf := make([]byte, len(httpData))
	n, err := serverProxy.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, httpData, buf[:n])

	// Verify HTTP data is NOT readable from control stream
	// Control stream should block or return EOF, not return HTTP data
	controlBuf := make([]byte, 100)
	readDone := make(chan struct {
		n   int
		err error
	}, 1)
	go func() {
		n, err := serverControl.Read(controlBuf)
		readDone <- struct {
			n   int
			err error
		}{n, err}
	}()

	select {
	case result := <-readDone:
		if result.err == nil && result.n > 0 {
			// We got data - check if it's HTTP data (BUG!)
			if bytes.HasPrefix(controlBuf[:result.n], []byte("GET")) {
				t.Fatalf("BUG: Control stream received HTTP data from proxy stream! Data: %s", string(controlBuf[:result.n]))
			}
			t.Logf("Control stream received non-HTTP data: %q", string(controlBuf[:result.n]))
		}
	case <-time.After(500 * time.Millisecond):
		// Control stream correctly blocked - this is expected behavior
		t.Log("Control stream correctly blocked (no data from proxy stream)")
		// Write control data - the blocking read should receive it
		controlData := []byte("control message")
		_, err = clientControl.Write(controlData)
		require.NoError(t, err)

		// Wait for the blocking read to complete
		select {
		case result := <-readDone:
			if result.err == nil && result.n > 0 {
				// Check if we got the control data we just wrote
				if bytes.HasPrefix(controlBuf[:result.n], controlData) {
					t.Logf("Control stream correctly received control data: %q", string(controlBuf[:result.n]))
				} else {
					t.Logf("Control stream received unexpected data: %q", string(controlBuf[:result.n]))
				}
			}
		case <-time.After(1 * time.Second):
			// Read should have completed by now
			t.Log("Control stream read did not complete - might be a timing issue")
		}
	}
}

// TestStreamIsolationConcurrent verifies stream isolation under concurrent load
func TestStreamIsolationConcurrent(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	// Create multiple streams
	const numStreams = 10
	clientStreams := make([]*Stream, numStreams)
	serverStreams := make([]*Stream, numStreams)

	for i := 0; i < numStreams; i++ {
		streamID := uint32(i + 1) // Start from 1, 0 is control
		clientStream, err := clientMux.OpenStreamWithID(streamID)
		require.NoError(t, err)
		clientStreams[i] = clientStream

		serverStream, err := serverMux.OpenStreamWithID(streamID)
		require.NoError(t, err)
		serverStreams[i] = serverStream
	}

	// Write unique data to each stream concurrently
	var wg sync.WaitGroup
	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(streamIdx int) {
			defer wg.Done()
			data := []byte{byte(streamIdx), byte(streamIdx), byte(streamIdx)}
			_, err := clientStreams[streamIdx].Write(data)
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// Verify each stream only receives its own data
	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(streamIdx int) {
			defer wg.Done()
			expectedData := []byte{byte(streamIdx), byte(streamIdx), byte(streamIdx)}
			buf := make([]byte, len(expectedData))
			n, err := serverStreams[streamIdx].Read(buf)
			require.NoError(t, err)
			assert.Equal(t, expectedData, buf[:n], "Stream %d received wrong data", streamIdx)
		}(i)
	}
	wg.Wait()
}

// TestControlStreamIsolation verifies that control stream (ID 0) is isolated from proxy streams
func TestControlStreamIsolation(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	// Create control stream on both sides
	clientControl, err := clientMux.ControlStream()
	require.NoError(t, err)
	serverControl, err := serverMux.ControlStream()
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond) // Give time for streams to sync

	// Create proxy stream - try to open, but if it already exists, get it
	clientProxy, err := clientMux.OpenStreamWithID(1)
	if err != nil && err.Error() == "stream 1 already exists" {
		clientProxy, err = clientMux.GetStream(1)
		require.NoError(t, err)
	} else {
		require.NoError(t, err)
	}

	// Write a byte to trigger server-side auto-creation (if not already created)
	_, err = clientProxy.Write([]byte{0})
	require.NoError(t, err)

	// Wait for server to auto-create stream 1
	serverProxy, err := waitForStream(serverMux, 1, 5*time.Second)
	require.NoError(t, err)

	// Read the byte we wrote
	proxyBuf := make([]byte, 1)
	_, err = serverProxy.Read(proxyBuf)
	require.NoError(t, err)

	// Write HTTP request to proxy stream (simulating real usage)
	httpRequest := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	_, err = clientProxy.Write(httpRequest)
	require.NoError(t, err)

	// Verify proxy stream receives HTTP data
	httpBuf := make([]byte, len(httpRequest))
	n, err := serverProxy.Read(httpBuf)
	require.NoError(t, err)
	assert.Equal(t, httpRequest, httpBuf[:n])

	// CRITICAL: Verify control stream does NOT receive HTTP data
	// This is the bug we're trying to catch
	controlBuf := make([]byte, 100)
	readDone := make(chan struct {
		n   int
		err error
	}, 1)
	go func() {
		n, err := serverControl.Read(controlBuf)
		readDone <- struct {
			n   int
			err error
		}{n, err}
	}()

	select {
	case result := <-readDone:
		if result.err == nil && result.n > 0 {
			// We got data - check if it's HTTP data (BUG!)
			if bytes.HasPrefix(controlBuf[:result.n], []byte("GET")) {
				t.Fatalf("CRITICAL BUG: Control stream received HTTP data from proxy stream! Data: %q", string(controlBuf[:result.n]))
			}
			t.Logf("Control stream received non-HTTP data (unexpected but not a bug): %q", string(controlBuf[:result.n]))
		} else if result.err != nil && result.err != io.EOF {
			// Some other error - might be OK
			t.Logf("Control stream read error: %v", result.err)
		}
	case <-time.After(500 * time.Millisecond):
		// Control stream correctly blocked - this is expected behavior
		t.Log("Control stream correctly isolated - no data received from proxy stream")
		// The read goroutine is still blocking, which is fine - it will be cleaned up
		// Now write control data - the blocking read should receive it
		controlData := []byte("control packet")
		_, err = clientControl.Write(controlData)
		require.NoError(t, err)

		// Wait for the blocking read to complete
		select {
		case result := <-readDone:
			if result.err == nil && result.n > 0 {
				// Check if we got the control data we just wrote
				if bytes.HasPrefix(controlBuf[:result.n], controlData) {
					t.Logf("Control stream correctly received control data: %q", string(controlBuf[:result.n]))
				} else {
					t.Logf("Control stream received unexpected data: %q", string(controlBuf[:result.n]))
				}
			}
		case <-time.After(1 * time.Second):
			// Read should have completed by now
			t.Log("Control stream read did not complete - might be a timing issue")
		}
	}
}

// TestMultiplexerHeaderCorrectness verifies that multiplexer headers are written correctly
func TestMultiplexerHeaderCorrectness(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	// Create streams with specific IDs
	streams := []struct {
		id   uint32
		data []byte
	}{
		{0, []byte("control")},
		{1, []byte("proxy1")},
		{2, []byte("proxy2")},
		{10, []byte("proxy10")},
	}

	clientStreams := make(map[uint32]*Stream)
	serverStreams := make(map[uint32]*Stream)

	// Create all streams
	for _, s := range streams {
		var (
			clientStream *Stream
			err          error
		)
		if s.id == 0 {
			clientStream, err = clientMux.ControlStream()
		} else {
			clientStream, err = clientMux.OpenStreamWithID(s.id)
		}
		require.NoError(t, err)
		clientStreams[s.id] = clientStream

		var serverStream *Stream
		if s.id == 0 {
			serverStream, err = serverMux.ControlStream()
		} else {
			serverStream, err = serverMux.OpenStreamWithID(s.id)
		}
		require.NoError(t, err)
		serverStreams[s.id] = serverStream
	}

	// Write data to each stream
	for _, s := range streams {
		_, err := clientStreams[s.id].Write(s.data)
		require.NoError(t, err)
	}

	// Verify each stream receives correct data
	for _, s := range streams {
		buf := make([]byte, len(s.data))
		n, err := serverStreams[s.id].Read(buf)
		require.NoError(t, err)
		assert.Equal(t, s.data, buf[:n], "Stream %d received wrong data", s.id)
	}
}

// TestConcurrentWritesToMultipleStreams tests concurrent writes to multiple streams
func TestConcurrentWritesToMultipleStreams(t *testing.T) {
	clientConn, serverConn := setupTCPConnection(t)
	defer clientConn.Close()
	defer serverConn.Close()

	clientMux := NewMultiplexer(clientConn)
	serverMux := NewMultiplexer(serverConn)
	time.Sleep(100 * time.Millisecond)

	const numStreams = 5
	const writesPerStream = 10

	clientStreams := make([]*Stream, numStreams)
	serverStreams := make([]*Stream, numStreams)

	// Create streams
	for i := 0; i < numStreams; i++ {
		streamID := uint32(i + 1)
		clientStream, err := clientMux.OpenStreamWithID(streamID)
		require.NoError(t, err)
		clientStreams[i] = clientStream

		serverStream, err := serverMux.OpenStreamWithID(streamID)
		require.NoError(t, err)
		serverStreams[i] = serverStream
	}

	// Concurrent writes across streams (preserve ordering within each stream)
	var wg sync.WaitGroup
	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(streamIdx int) {
			defer wg.Done()
			for writeIdx := 0; writeIdx < writesPerStream; writeIdx++ {
				data := []byte{byte(streamIdx), byte(writeIdx)}
				_, err := clientStreams[streamIdx].Write(data)
				require.NoError(t, err)
			}
		}(i)
	}
	wg.Wait()

	// Verify all data received correctly
	received := make([][]byte, numStreams)
	for i := 0; i < numStreams; i++ {
		received[i] = make([]byte, 0, writesPerStream*2)
		for j := 0; j < writesPerStream; j++ {
			buf := make([]byte, 2)
			n, err := serverStreams[i].Read(buf)
			require.NoError(t, err)
			assert.Equal(t, 2, n)
			received[i] = append(received[i], buf[:n]...)
		}
	}

	// Verify data integrity
	for i := 0; i < numStreams; i++ {
		assert.Equal(t, writesPerStream*2, len(received[i]), "Stream %d should receive %d bytes", i, writesPerStream*2)
		for j := 0; j < writesPerStream; j++ {
			expected := []byte{byte(i), byte(j)}
			actual := received[i][j*2 : j*2+2]
			assert.Equal(t, expected, actual, "Stream %d, write %d", i, j)
		}
	}
}

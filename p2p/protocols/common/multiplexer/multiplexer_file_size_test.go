package multiplexer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiplexerLargeWrites tests writing various sizes through the multiplexer
// This helps identify if the issue is in the multiplexer layer
func TestMultiplexerLargeWrites(t *testing.T) {
	c1, c2 := net.Pipe()

	mux1 := NewMultiplexer(c1)
	mux2 := NewMultiplexer(c2)
	require.NotNil(t, mux1)
	require.NotNil(t, mux2)

	defer mux1.Close()
	defer mux2.Close()

	// Test sizes around the problematic threshold
	testSizes := []int{
		100,
		1024,
		32768,   // 32KB
		65536,   // 64KB
		131072,  // 128KB
		256000,  // ~250KB
		261460,  // Just below threshold
		261461,  // Just below threshold
		261462,  // Exact threshold (reported working)
		261463,  // Just above threshold (reported failing)
		261464,  // Just above threshold
		262144,  // 256KB (2^18)
		524288,  // 512KB
		1048576, // 1MB
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("Write_%d_bytes", size), func(t *testing.T) {
			// Create streams
			stream1, err := mux1.OpenStream()
			require.NoError(t, err)

			// Write a small amount of data to trigger stream creation on the other side
			// Streams are auto-created when data arrives via readLoop
			_, err = stream1.Write([]byte{0})
			require.NoError(t, err)

			// Wait for the stream to be created on the other side
			// Poll with timeout to ensure stream is created
			var stream2 *Stream
			deadline := time.Now().Add(1 * time.Second)
			for time.Now().Before(deadline) {
				stream2, err = mux2.GetStream(stream1.ID)
				if err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			require.NoError(t, err, "Stream should be created on the other side")
			require.NotNil(t, stream2, "Stream should not be nil")

			// Read the initial byte we wrote
			buf := make([]byte, 1)
			_, err = stream2.Read(buf)
			require.NoError(t, err)

			// Generate test data
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}

			// Write data in a goroutine
			writeErr := make(chan error, 1)
			go func() {
				_, err := stream1.Write(data)
				writeErr <- err
			}()

			// Read data
			readData := make([]byte, size)
			readErr := make(chan error, 1)
			go func() {
				_, err := io.ReadFull(stream2, readData)
				readErr <- err
			}()

			// Wait for both to complete
			select {
			case err := <-writeErr:
				if err != nil {
					t.Logf("Write failed for size %d: %v", size, err)
					t.Logf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
				}
				require.NoError(t, err, "Write failed for size %d", size)
			case <-time.After(5 * time.Second):
				t.Fatalf("Write timeout for size %d", size)
			}

			select {
			case err := <-readErr:
				if err != nil {
					t.Logf("Read failed for size %d: %v", size, err)
					t.Logf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
				}
				require.NoError(t, err, "Read failed for size %d", size)
			case <-time.After(5 * time.Second):
				t.Fatalf("Read timeout for size %d", size)
			}

			// Verify data
			assert.Equal(t, data, readData, "Data mismatch for size %d", size)

			stream1.Close()
			stream2.Close()
		})
	}
}

// TestMultiplexerWriteDataDirectly tests writeData function directly with various sizes
func TestMultiplexerWriteDataDirectly(t *testing.T) {
	c1, c2 := net.Pipe()

	mux := NewMultiplexer(c1)
	require.NotNil(t, mux)
	defer mux.Close()

	// Test sizes around threshold
	testSizes := []int{
		261460,
		261461,
		261462,
		261463,
		261464,
		262144,
	}

	// Read on the other end - need to consume from the multiplexer's readLoop
	// The readLoop reads from c1 and delivers to streams, so we need to read from c2
	// which is the other end of the pipe
	readDone := make(chan error, 1)
	go func() {
		header := make([]byte, HeaderSize)
		for i, size := range testSizes {
			// Read header
			if _, err := io.ReadFull(c2, header); err != nil {
				readDone <- err
				return
			}

			_ = binary.BigEndian.Uint32(header[0:4]) // streamID (not used in this test)
			length := binary.BigEndian.Uint32(header[4:8])

			// Check for EOF marker (zero-length frame) after all data frames
			if length == 0 {
				// This is the EOF marker - we've read all data frames
				if i < len(testSizes)-1 {
					readDone <- fmt.Errorf("unexpected EOF marker at index %d, expected %d more frames", i, len(testSizes)-i-1)
					return
				}
				// All data frames read, EOF marker received - done
				readDone <- nil
				return
			}

			if length != uint32(size) {
				readDone <- fmt.Errorf("length mismatch: expected %d, got %d", size, length)
				return
			}

			// Read data
			data := make([]byte, length)
			if _, err := io.ReadFull(c2, data); err != nil {
				readDone <- err
				return
			}
		}
		// If we get here, we need to read the EOF marker
		// Read one more frame for EOF marker
		if _, err := io.ReadFull(c2, header); err != nil {
			readDone <- err
			return
		}
		length := binary.BigEndian.Uint32(header[4:8])
		if length != 0 {
			readDone <- fmt.Errorf("expected EOF marker (length 0), got %d", length)
			return
		}
		readDone <- nil
	}()

	// Write data through stream
	stream, err := mux.OpenStream()
	require.NoError(t, err)

	for _, size := range testSizes {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i % 256)
		}

		_, err := stream.Write(data)
		if err != nil {
			t.Logf("Direct write failed for size %d: %v", size, err)
			t.Logf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
		}
		require.NoError(t, err, "Direct write failed for size %d", size)
	}

	// Close the stream - this sends EOF marker
	stream.Close()

	// Wait for read to complete with timeout
	select {
	case err := <-readDone:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Read timeout - readLoop may be blocked")
	}
}

// TestMultiplexerIoCopy tests io.Copy behavior with various buffer sizes
func TestMultiplexerIoCopy(t *testing.T) {
	c1, c2 := net.Pipe()

	mux1 := NewMultiplexer(c1)
	mux2 := NewMultiplexer(c2)
	require.NotNil(t, mux1)
	require.NotNil(t, mux2)

	defer mux1.Close()
	defer mux2.Close()

	testSizes := []int64{
		261460,
		261461,
		261462,
		261463,
		261464,
		262144,
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("IoCopy_%d_bytes", size), func(t *testing.T) {
			stream1, err := mux1.OpenStream()
			require.NoError(t, err)

			// Write a small amount of data to trigger stream creation on the other side
			// Streams are auto-created when data arrives via readLoop
			_, err = stream1.Write([]byte{0})
			require.NoError(t, err)

			// Wait for the stream to be created on the other side
			// Poll with timeout to ensure stream is created
			var stream2 *Stream
			deadline := time.Now().Add(1 * time.Second)
			for time.Now().Before(deadline) {
				stream2, err = mux2.GetStream(stream1.ID)
				if err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			require.NoError(t, err, "Stream should be created on the other side")
			require.NotNil(t, stream2, "Stream should not be nil")

			// Read the initial byte we wrote
			buf := make([]byte, 1)
			_, err = stream2.Read(buf)
			require.NoError(t, err)

			// Create source data
			srcData := make([]byte, size)
			for i := range srcData {
				srcData[i] = byte(i % 256)
			}
			src := bytes.NewReader(srcData)

			// Copy data
			copyErr := make(chan error, 1)
			go func() {
				_, err := io.Copy(stream1, src)
				copyErr <- err
			}()

			// Read all data
			dstData := make([]byte, size)
			readErr := make(chan error, 1)
			go func() {
				_, err := io.ReadFull(stream2, dstData)
				readErr <- err
			}()

			// Wait for copy
			select {
			case err := <-copyErr:
				if err != nil {
					t.Logf("io.Copy failed for size %d: %v", size, err)
					t.Logf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
				}
				require.NoError(t, err, "io.Copy failed for size %d", size)
			case <-time.After(10 * time.Second):
				t.Fatalf("io.Copy timeout for size %d", size)
			}

			// Wait for read
			select {
			case err := <-readErr:
				if err != nil {
					t.Logf("Read failed for size %d: %v", size, err)
					t.Logf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
				}
				require.NoError(t, err, "Read failed for size %d", size)
			case <-time.After(10 * time.Second):
				t.Fatalf("Read timeout for size %d", size)
			}

			// Verify
			assert.Equal(t, srcData, dstData, "Data mismatch for size %d", size)

			stream1.Close()
			stream2.Close()
		})
	}
}

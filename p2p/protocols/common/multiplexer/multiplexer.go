package multiplexer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var (
	ErrStreamNotFound  = errors.New("stream not found")
	ErrStreamClosed    = errors.New("stream closed")
	ErrInvalidStreamID = errors.New("invalid stream ID")
)

const (
	// HeaderSize is the size of the multiplexer header (stream ID + length)
	HeaderSize = 8
	// MaxStreamID is the maximum stream ID
	MaxStreamID = 0xFFFFFFFF
)

// Stream represents a single multiplexed stream
type Stream struct {
	ID     uint32
	reader *streamReader
	writer *streamWriter
	closed chan struct{}
	mu     sync.Mutex
}

// Read reads data from the stream
func (s *Stream) Read(p []byte) (n int, err error) {
	return s.reader.Read(p)
}

// Write writes data to the stream
func (s *Stream) Write(p []byte) (n int, err error) {
	return s.writer.Write(p)
}

// Close closes the stream
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.closed:
		return ErrStreamClosed
	default:
		close(s.closed)
		s.reader.Close()
		s.writer.Close()
		return nil
	}
}

// IsClosed returns whether the stream is closed
func (s *Stream) IsClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

type streamReader struct {
	id       uint32
	dataChan chan []byte
	errChan  chan error
	closed   bool
	mu       sync.Mutex
	buffer   []byte
}

func newStreamReader(id uint32) *streamReader {
	// Use a very large buffered channel to prevent readLoop from blocking
	// Buffer size of 10000 should be sufficient for most use cases, including large HTTP responses
	// This allows readLoop to continue reading even if the consumer is slow
	// Each entry in the channel can be a full multiplexer packet (up to 4GB), so this gives us
	// a lot of headroom for buffering data before blocking readLoop
	return &streamReader{
		id:       id,
		dataChan: make(chan []byte, 10000),
		errChan:  make(chan error, 1),
	}
}

func (sr *streamReader) Read(p []byte) (n int, err error) {
	// First, try to read from buffer (without lock for performance)
	sr.mu.Lock()
	if len(sr.buffer) > 0 {
		n = copy(p, sr.buffer)
		sr.buffer = sr.buffer[n:]
		closed := sr.closed
		sr.mu.Unlock()
		if closed {
			return 0, io.EOF
		}
		return n, nil
	}
	closed := sr.closed
	sr.mu.Unlock()

	if closed {
		return 0, io.EOF
	}

	// Wait for data or error (without holding lock)
	select {
	case data := <-sr.dataChan:
		if len(data) == 0 {
			return 0, io.EOF
		}
		n = copy(p, data)
		if n < len(data) {
			// Buffer the remainder (need lock for this)
			sr.mu.Lock()
			sr.buffer = data[n:]
			sr.mu.Unlock()
		}
		return n, nil
	case err := <-sr.errChan:
		return 0, err
	}
}

func (sr *streamReader) WriteData(data []byte) {
	sr.mu.Lock()
	closed := sr.closed
	sr.mu.Unlock()

	if closed {
		return
	}

	// We must block here to ensure data is delivered
	// However, if the channel is full, this blocks readLoop, which can cause deadlock
	// The solution is to ensure the buffer is large enough (10000) and that the consumer
	// (io.Copy) is actively reading. If the channel fills up, it means the consumer is
	// severely backlogged, which is a serious issue.
	//
	// For now, we block to ensure data delivery. The large buffer (10000) should prevent
	// this from happening under normal circumstances.
	sr.dataChan <- data
}

func (sr *streamReader) WriteError(err error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if !sr.closed {
		select {
		case sr.errChan <- err:
		default:
		}
	}
}

func (sr *streamReader) Close() {
	sr.mu.Lock()
	wasClosed := sr.closed
	sr.closed = true
	sr.mu.Unlock()

	if !wasClosed {
		// Send EOF to error channel to unblock any waiting Read() calls
		// This must be done before closing channels to ensure the select in Read() sees it
		select {
		case sr.errChan <- io.EOF:
		default:
			// Channel might be full or already have an error, that's OK
			// Closing dataChan will still unblock the select
		}

		// Close dataChan - reading from a closed channel returns zero value immediately
		// This will cause the select in Read() to return with len(data) == 0, which returns EOF
		close(sr.dataChan)

		// Close errChan - this is safe because we already sent EOF above
		close(sr.errChan)
	}
}

type streamWriter struct {
	id      uint32
	writeFn func(uint32, []byte) error
	closed  bool
	mu      sync.Mutex
}

func newStreamWriter(id uint32, writeFn func(uint32, []byte) error) *streamWriter {
	return &streamWriter{
		id:      id,
		writeFn: writeFn,
	}
}

func (sw *streamWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.closed {
		return 0, io.ErrClosedPipe
	}

	if err := sw.writeFn(sw.id, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

func (sw *streamWriter) Close() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.closed = true
}

// Multiplexer handles connection multiplexing
type Multiplexer struct {
	conn    io.ReadWriteCloser
	streams map[uint32]*Stream
	nextID  uint32
	mu      sync.RWMutex // Protects streams map and closed flag
	writeMu sync.Mutex   // Serializes writes to conn to prevent data corruption
	closed  bool
	readErr error
	readWg  sync.WaitGroup
}

// NewMultiplexer creates a new multiplexer
func NewMultiplexer(conn io.ReadWriteCloser) *Multiplexer {
	if conn == nil {
		return nil
	}
	m := &Multiplexer{
		conn:    conn,
		streams: make(map[uint32]*Stream),
		nextID:  1,
	}

	m.readWg.Add(1)
	go m.readLoop()

	return m
}

// OpenStream opens a new stream with auto-assigned ID
func (m *Multiplexer) OpenStream() (*Stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, io.ErrClosedPipe
	}

	for m.streams[m.nextID] != nil {
		m.nextID++
		if m.nextID > MaxStreamID {
			m.nextID = 1
		}
		if m.nextID == 0 {
			m.nextID = 1
		}
	}
	id := m.nextID
	m.nextID++

	stream := m.newStream(id)
	m.streams[id] = stream
	return stream, nil
}

// ControlStream returns the reserved stream (ID 0) used for control-plane traffic.
func (m *Multiplexer) ControlStream() (*Stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, io.ErrClosedPipe
	}

	if stream, ok := m.streams[0]; ok {
		return stream, nil
	}

	stream := m.newStream(0)
	m.streams[0] = stream
	return stream, nil
}

// OpenStreamWithID opens a stream with a specific ID.
func (m *Multiplexer) OpenStreamWithID(requestedID uint32) (*Stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, io.ErrClosedPipe
	}

	if requestedID == 0 {
		return nil, ErrInvalidStreamID
	}

	if m.streams[requestedID] != nil {
		return nil, fmt.Errorf("stream %d already exists", requestedID)
	}

	stream := m.newStream(requestedID)
	m.streams[requestedID] = stream

	if requestedID >= m.nextID {
		m.nextID = requestedID + 1
	}

	return stream, nil
}

func (m *Multiplexer) newStream(id uint32) *Stream {
	return &Stream{
		ID:     id,
		reader: newStreamReader(id),
		writer: newStreamWriter(id, m.writeData),
		closed: make(chan struct{}),
	}
}

// CloseStream closes a stream
func (m *Multiplexer) CloseStream(id uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stream, ok := m.streams[id]
	if !ok {
		return ErrStreamNotFound
	}

	delete(m.streams, id)
	return stream.Close()
}

// GetStream gets an existing stream
func (m *Multiplexer) GetStream(id uint32) (*Stream, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stream, ok := m.streams[id]
	if !ok {
		return nil, ErrStreamNotFound
	}

	return stream, nil
}

func (m *Multiplexer) writeData(streamID uint32, data []byte) error {
	// CRITICAL: Validate stream ID - stream 0 is reserved for control
	// If HTTP data (starts with "GET ", "POST", etc.) is written with stream ID 0,
	// it will be read by the control stream, causing "packet too large" errors
	if streamID == 0 && len(data) > 0 {
		// Check if this looks like HTTP data being written to control stream
		if len(data) >= 4 {
			httpPrefix := string(data[:4])
			if httpPrefix == "GET " || httpPrefix == "POST" || httpPrefix == "PUT " ||
				httpPrefix == "HEAD" || httpPrefix == "DELE" || httpPrefix == "OPTI" {
				// This is a critical bug - log it and return error
				return fmt.Errorf("CRITICAL BUG: HTTP data written to stream ID 0 (control stream). HTTP data should be written to proxy streams (ID > 0). Data starts with: %q. This indicates the client is using the wrong stream for HTTP data.", httpPrefix)
			}
		}
	}

	// Debug: Log if we're writing HTTP-like data to any stream (for debugging)
	if len(data) >= 4 {
		prefix := string(data[:4])
		if prefix == "GET " || prefix == "POST" || prefix == "PUT " ||
			prefix == "HEAD" || prefix == "DELE" || prefix == "OPTI" {
			// This is HTTP data - verify it's NOT going to stream 0
			if streamID == 0 {
				return fmt.Errorf("CRITICAL BUG: HTTP data written to stream ID 0 (control stream). Stream ID: %d, Data prefix: %q", streamID, prefix)
			}
		}
	}

	// Check if closed (read lock is sufficient for this check)
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()

	if closed {
		return io.ErrClosedPipe
	}

	// Serialize writes to the underlying connection to prevent data corruption
	// Multiple streams can write concurrently, but the actual network writes must be serialized
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	// Write header: stream ID (4 bytes) + length (4 bytes)
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], streamID)
	binary.BigEndian.PutUint32(header[4:8], uint32(len(data)))

	if _, err := m.conn.Write(header); err != nil {
		return err
	}

	if _, err := m.conn.Write(data); err != nil {
		return err
	}

	return nil
}

func (m *Multiplexer) readLoop() {
	defer m.readWg.Done()

	header := make([]byte, HeaderSize)
	for {
		// Read header
		if _, err := io.ReadFull(m.conn, header); err != nil {
			m.mu.Lock()
			m.readErr = err
			m.closed = true
			m.mu.Unlock()

			// Notify all streams
			m.mu.RLock()
			for _, stream := range m.streams {
				stream.reader.WriteError(err)
			}
			m.mu.RUnlock()
			return
		}

		streamID := binary.BigEndian.Uint32(header[0:4])
		length := binary.BigEndian.Uint32(header[4:8])

		// CRITICAL: Validate length to detect HTTP data being read as packet length
		// If we see "GET " (0x47455420) as a length, it means HTTP data is being read
		// from the wrong stream (likely control stream reading proxy stream data)
		if length == 0x47455420 {
			// This is "GET " in ASCII - HTTP data is being read as a packet length
			// This indicates a serious routing issue
			m.mu.Lock()
			m.readErr = fmt.Errorf("CRITICAL BUG: Multiplexer readLoop detected HTTP data ('GET ') being read as packet length. Stream ID: %d, Length: 0x%08X. This indicates HTTP data is being routed to stream %d when it should be routed to a proxy stream (ID > 0)", streamID, length, streamID)
			m.closed = true
			m.mu.Unlock()
			return
		}

		// Read data
		data := make([]byte, length)
		if _, err := io.ReadFull(m.conn, data); err != nil {
			m.mu.Lock()
			m.readErr = err
			m.closed = true
			m.mu.Unlock()
			return
		}

		// CRITICAL: If this is stream 0 and the data looks like HTTP, it's a bug
		if streamID == 0 && len(data) >= 4 {
			httpPrefix := string(data[:4])
			if httpPrefix == "GET " || httpPrefix == "POST" || httpPrefix == "PUT " ||
				httpPrefix == "HEAD" || httpPrefix == "DELE" || httpPrefix == "OPTI" {
				// HTTP data is being delivered to control stream - this is a critical bug
				m.mu.Lock()
				m.readErr = fmt.Errorf("CRITICAL BUG: Multiplexer readLoop detected HTTP data being delivered to control stream (ID 0). Data starts with: %q. This indicates HTTP data is being written with stream ID 0 when it should be written to a proxy stream (ID > 0)", httpPrefix)
				m.closed = true
				m.mu.Unlock()
				return
			}
		}

		// Deliver to stream
		m.mu.Lock()
		stream, ok := m.streams[streamID]
		if !ok {
			// Auto-create stream if it doesn't exist (for server-side stream acceptance)
			stream = &Stream{
				ID:     streamID,
				reader: newStreamReader(streamID),
				writer: newStreamWriter(streamID, m.writeData),
				closed: make(chan struct{}),
			}
			m.streams[streamID] = stream
		}
		m.mu.Unlock()

		if stream != nil {
			stream.reader.WriteData(data)
		}
	}
}

// Close closes the multiplexer and all streams
func (m *Multiplexer) Close() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	streams := make([]*Stream, 0, len(m.streams))
	for _, stream := range m.streams {
		streams = append(streams, stream)
	}
	conn := m.conn
	m.mu.Unlock()

	// Close underlying connection first to unblock readLoop
	var connErr error
	if conn != nil {
		connErr = conn.Close()
	}

	// Close all streams
	for _, stream := range streams {
		stream.Close()
	}

	// Wait for readLoop to finish (with timeout to prevent hanging)
	done := make(chan struct{})
	go func() {
		m.readWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// readLoop finished
	case <-time.After(5 * time.Second):
		// Timeout - readLoop didn't finish, but we'll continue anyway
	}

	return connErr
}

// IsClosed returns true if the multiplexer is closed
func (m *Multiplexer) IsClosed() bool {
	if m == nil {
		return true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// StreamCount returns the number of active streams
func (m *Multiplexer) StreamCount() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.streams)
}

// HasStream returns true if a stream with the given ID exists
func (m *Multiplexer) HasStream(id uint32) bool {
	if m == nil {
		return false
	}
	_, err := m.GetStream(id)
	return err == nil
}

// ReadError returns the error from the read loop, if any
func (m *Multiplexer) ReadError() error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.readErr
}

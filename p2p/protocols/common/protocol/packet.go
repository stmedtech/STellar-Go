package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// PacketType represents the type of packet in the protocol
type PacketType string

const (
	PacketTypeTunneling PacketType = "tunneling"
	PacketTypeHandshake PacketType = "handshake"
	PacketTypeStatus    PacketType = "status"
	PacketTypeNested    PacketType = "nested"
)

// Packet represents the base packet structure
type Packet struct {
	Type    PacketType      `json:"type"`
	Content json.RawMessage `json:"content"`
}

// TunnelingContent represents the content structure for tunneling packets
type TunnelingContent struct {
	Data []byte `json:"data"`
}

// Marshal serializes the packet to JSON bytes
func (p *Packet) Marshal() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("packet is nil")
	}
	return json.Marshal(p)
}

// Unmarshal deserializes JSON bytes into the packet
func (p *Packet) Unmarshal(data []byte) error {
	if p == nil {
		return fmt.Errorf("packet is nil")
	}
	return json.Unmarshal(data, p)
}

// NewTunnelingPacket creates a new tunneling packet with the given data
func NewTunnelingPacket(data []byte) (*Packet, error) {
	if data == nil {
		data = []byte{}
	}
	content := TunnelingContent{Data: data}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("marshal content: %w", err)
	}

	return &Packet{
		Type:    PacketTypeTunneling,
		Content: contentJSON,
	}, nil
}

// UnmarshalTunnelingContent extracts the tunneling content from a packet
func (p *Packet) UnmarshalTunnelingContent() (*TunnelingContent, error) {
	if p == nil {
		return nil, fmt.Errorf("packet is nil")
	}
	if p.Type != PacketTypeTunneling {
		return nil, fmt.Errorf("not a tunneling packet: %s", p.Type)
	}

	var content TunnelingContent
	if err := json.Unmarshal(p.Content, &content); err != nil {
		return nil, fmt.Errorf("unmarshal content: %w", err)
	}

	return &content, nil
}

// WritePacket writes a packet to an io.Writer with length prefix
func WritePacket(w io.Writer, packet *Packet) error {
	if packet == nil {
		return fmt.Errorf("packet is nil")
	}
	if w == nil {
		return fmt.Errorf("writer is nil")
	}

	data, err := packet.Marshal()
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	length := uint32(len(data))

	// Write length prefix (4 bytes, big-endian)
	if err := writeUint32(w, length); err != nil {
		return fmt.Errorf("write length: %w", err)
	}

	// Write packet data
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}

// ReadPacket reads a packet from an io.Reader with length prefix
func ReadPacket(r io.Reader) (*Packet, error) {
	if r == nil {
		return nil, fmt.Errorf("reader is nil")
	}

	// Read length prefix
	length, err := readUint32(r)
	if err != nil {
		return nil, fmt.Errorf("read length: %w", err)
	}

	// Validate length (prevent excessive memory allocation)
	const maxPacketSize = 1024 * 1024 * 1024         // 1GB max
	const reasonableMaxPacketSize = 10 * 1024 * 1024 // 10MB reasonable max for control messages

	// CRITICAL: Check if we're reading HTTP data instead of packet data
	// The value 1195725856 (0x47455420) is "GET " in ASCII
	// This indicates HTTP request data is being read from the control stream
	// This is a serious bug - HTTP data should never be on the control stream
	if length == 0x47455420 {
		return nil, fmt.Errorf("packet too large: %d bytes (0x%08X = 'GET ') - CRITICAL BUG: reading HTTP request data from control stream instead of packet data. This indicates HTTP data is being written to stream ID 0 (control stream) when it should be written to a proxy stream. Check client proxy stream creation.", length, length)
	}

	if length > maxPacketSize {
		return nil, fmt.Errorf("packet too large: %d bytes (max: 1GB)", length)
	}

	// Additional validation: if length is unreasonably large, it might be multiplexer frame data
	// or we're reading HTTP data instead of packet data.
	// For control messages, they should be small (< 10MB). If we get something larger,
	// it's likely we're reading from the wrong stream or reading corrupted data.
	if length > reasonableMaxPacketSize {
		return nil, fmt.Errorf("packet too large for control message: %d bytes (likely reading from wrong stream or corrupted data)", length)
	}

	// Read packet data
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	// Unmarshal packet
	var packet Packet
	if err := packet.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &packet, nil
}

// writeUint32 writes a uint32 in big-endian format
func writeUint32(w io.Writer, v uint32) error {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	_, err := w.Write(buf)
	return err
}

// readUint32 reads a uint32 in big-endian format
func readUint32(r io.Reader) (uint32, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf), nil
}

// PacketReadWriteCloser wraps an io.ReadWriteCloser for packet-based communication
type PacketReadWriteCloser struct {
	conn    io.ReadWriteCloser
	writeMu sync.Mutex // Serializes WritePacket calls to prevent packet corruption
}

// NewPacketReadWriteCloser creates a new PacketReadWriteCloser
func NewPacketReadWriteCloser(conn io.ReadWriteCloser) *PacketReadWriteCloser {
	if conn == nil {
		return nil
	}
	return &PacketReadWriteCloser{conn: conn}
}

// Read reads data from the underlying connection.
// For packet-based communication, use ReadPacket() instead.
func (p *PacketReadWriteCloser) Read(data []byte) (int, error) {
	if p == nil || p.conn == nil {
		return 0, io.ErrClosedPipe
	}
	return p.conn.Read(data)
}

// Write writes data to the underlying connection.
// For packet-based communication, use WritePacket() instead.
func (p *PacketReadWriteCloser) Write(data []byte) (int, error) {
	if p == nil || p.conn == nil {
		return 0, io.ErrClosedPipe
	}
	return p.conn.Write(data)
}

// Close closes the underlying connection
func (p *PacketReadWriteCloser) Close() error {
	if p == nil || p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

// ReadPacket reads a packet from the connection
func (p *PacketReadWriteCloser) ReadPacket() (*Packet, error) {
	if p == nil || p.conn == nil {
		return nil, io.ErrClosedPipe
	}
	return ReadPacket(p.conn)
}

// WritePacket writes a packet to the connection
// This method is thread-safe - it serializes writes to prevent packet corruption
func (p *PacketReadWriteCloser) WritePacket(packet *Packet) error {
	if p == nil || p.conn == nil {
		return io.ErrClosedPipe
	}
	// Serialize writes to prevent interleaving of length prefix and data
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return WritePacket(p.conn, packet)
}

package handlers

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ProtocolFramer handles reading and writing complete protocol frames
type ProtocolFramer interface {
	// ReadFrame reads one complete protocol frame from the connection
	// Returns the complete frame bytes (including all headers)
	ReadFrame() ([]byte, error)

	// WriteFrame writes one complete frame to the connection
	WriteFrame(frame []byte) error
}

// MySQLFramer reads/writes MySQL protocol frames
type MySQLFramer struct {
	conn io.ReadWriter
	buf  []byte // pending data
}

// NewMySQLFramer creates a MySQL protocol framer
func NewMySQLFramer(conn io.ReadWriter) *MySQLFramer {
	return &MySQLFramer{
		conn: conn,
		buf:  make([]byte, 0, 65536),
	}
}

// ReadFrame reads one complete MySQL packet
// Format: 3-byte length (little-endian) + 1-byte sequence + payload
func (mf *MySQLFramer) ReadFrame() ([]byte, error) {
	for {
		// Try to parse a complete frame from current buffer
		if len(mf.buf) >= 4 {
			// We have at least the header
			length := int(mf.buf[0]) | (int(mf.buf[1]) << 8) | (int(mf.buf[2]) << 16)
			totalPacketSize := 4 + length // 3-byte length + 1-byte seq + payload

			if len(mf.buf) >= totalPacketSize {
				// We have a complete frame
				frame := mf.buf[:totalPacketSize]
				// Copy the frame and remove it from buffer
				result := make([]byte, totalPacketSize)
				copy(result, frame)
				mf.buf = mf.buf[totalPacketSize:]
				return result, nil
			}
		}

		// Need more data
		// Grow buffer if necessary
		if len(mf.buf)+4096 > cap(mf.buf) {
			newBuf := make([]byte, len(mf.buf), len(mf.buf)*2)
			copy(newBuf, mf.buf)
			mf.buf = newBuf
		}

		// Read more data
		n, err := mf.conn.Read(mf.buf[len(mf.buf):cap(mf.buf)])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.EOF
		}

		mf.buf = mf.buf[:len(mf.buf)+n]
	}
}

// WriteFrame writes a MySQL packet to the connection
func (mf *MySQLFramer) WriteFrame(frame []byte) error {
	n, err := mf.conn.Write(frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return fmt.Errorf("partial write: wrote %d of %d bytes", n, len(frame))
	}
	return nil
}

// PostgreSQLFramer reads/writes PostgreSQL protocol frames
type PostgreSQLFramer struct {
	conn io.ReadWriter
	buf  []byte // pending data
}

// NewPostgreSQLFramer creates a PostgreSQL protocol framer
func NewPostgreSQLFramer(conn io.ReadWriter) *PostgreSQLFramer {
	return &PostgreSQLFramer{
		conn: conn,
		buf:  make([]byte, 0, 65536),
	}
}

// ReadFrame reads one complete PostgreSQL message
// Format: 1-byte type + 4-byte length (big-endian, includes the 4-byte length field) + payload
func (pf *PostgreSQLFramer) ReadFrame() ([]byte, error) {
	for {
		// Try to parse a complete frame from current buffer
		if len(pf.buf) >= 5 {
			// We have at least type + length
			msgLen := int(binary.BigEndian.Uint32(pf.buf[1:5]))

			// msgLen is the total message size INCLUDING the 4-byte length field
			totalPacketSize := 1 + msgLen // type + (length field + payload)

			if len(pf.buf) >= totalPacketSize {
				// We have a complete frame
				frame := pf.buf[:totalPacketSize]
				// Copy the frame and remove it from buffer
				result := make([]byte, totalPacketSize)
				copy(result, frame)
				pf.buf = pf.buf[totalPacketSize:]
				return result, nil
			}
		}

		// Need more data
		// Grow buffer if necessary
		if len(pf.buf)+4096 > cap(pf.buf) {
			newBuf := make([]byte, len(pf.buf), len(pf.buf)*2)
			copy(newBuf, pf.buf)
			pf.buf = newBuf
		}

		// Read more data
		n, err := pf.conn.Read(pf.buf[len(pf.buf):cap(pf.buf)])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.EOF
		}

		pf.buf = pf.buf[:len(pf.buf)+n]
	}
}

// WriteFrame writes a PostgreSQL message to the connection
func (pf *PostgreSQLFramer) WriteFrame(frame []byte) error {
	n, err := pf.conn.Write(frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return fmt.Errorf("partial write: wrote %d of %d bytes", n, len(frame))
	}
	return nil
}

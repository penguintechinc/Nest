package protocol

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

// buildMySQLHandshakePayload crafts a minimal but valid server Handshake v10
// payload (mysql_native_password), matching parseHandshake's expected layout.
func buildMySQLHandshakePayload() []byte {
	var p []byte
	p = append(p, 0x0a)                      // protocol version
	p = append(p, []byte("8.0.0")...)        // server version
	p = append(p, 0)                         // null terminator
	p = append(p, 1, 0, 0, 0)                // connection id (LE)
	p = append(p, []byte("ABCDEFGH")...)     // auth-plugin-data part 1 (8 bytes)
	p = append(p, 0)                         // filler
	p = append(p, 0xff, 0xf7)                // capability flags lower
	p = append(p, 0x21)                      // charset (utf8)
	p = append(p, 0x02, 0x00)                // status flags
	p = append(p, 0x0f, 0x80)                // capability flags upper
	p = append(p, 21)                        // auth-plugin-data length (part2 = 13)
	p = append(p, make([]byte, 10)...)       // reserved
	p = append(p, []byte("IJKLMNOPQRST")...) // auth-plugin-data part 2 (12 bytes)
	p = append(p, 0)                         // null terminator (13th)
	p = append(p, []byte("mysql_native_password")...)
	p = append(p, 0)
	return p
}

// TestMySQLAuthExchange drives the client side of the MySQL auth handshake over
// net.Pipe against a scripted server: read handshake -> send response -> read OK.
func TestMySQLAuthExchange(t *testing.T) {
	client, server := net.Pipe()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	_ = server.SetDeadline(time.Now().Add(5 * time.Second))

	srvErr := make(chan error, 1)
	go func() {
		// 1. server -> client: handshake (seq 0)
		if _, err := server.Write(mysqlPacket(0, buildMySQLHandshakePayload())); err != nil {
			srvErr <- err
			return
		}
		// 2. client -> server: HandshakeResponse — drain 3-byte len + seq + payload
		hdr := make([]byte, 4)
		if _, err := io.ReadFull(server, hdr); err != nil {
			srvErr <- err
			return
		}
		n := int(hdr[0]) | int(hdr[1])<<8 | int(hdr[2])<<16
		if _, err := io.ReadFull(server, make([]byte, n)); err != nil {
			srvErr <- err
			return
		}
		// 3. server -> client: OK (seq 2)
		_, err := server.Write(mysqlPacket(2, []byte{0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}))
		srvErr <- err
	}()

	ah := NewMySQLAuthHandler("secret")
	hs, err := ah.HandleHandshake(client)
	if err != nil {
		t.Fatalf("HandleHandshake: %v", err)
	}
	if hs.ProtoVersion != 0x0a {
		t.Errorf("ProtoVersion = %d, want 10", hs.ProtoVersion)
	}
	if hs.ServerVersion != "8.0.0" {
		t.Errorf("ServerVersion = %q, want 8.0.0", hs.ServerVersion)
	}
	if hs.AuthPluginName != "mysql_native_password" {
		t.Errorf("AuthPluginName = %q", hs.AuthPluginName)
	}

	if err := ah.SendHandshakeResponse(client, "user", "appdb"); err != nil {
		t.Fatalf("SendHandshakeResponse: %v", err)
	}
	if err := ah.ReadAuthResult(client); err != nil {
		t.Fatalf("ReadAuthResult (OK expected): %v", err)
	}
	client.Close()
	if err := <-srvErr; err != nil {
		t.Fatalf("server side: %v", err)
	}
}

func TestSendHandshakeResponse_NoHandshake(t *testing.T) {
	ah := NewMySQLAuthHandler("p")
	if err := ah.SendHandshakeResponse(&bytes.Buffer{}, "u", "d"); err == nil {
		t.Error("expected error when handshake not yet received")
	}
}

func TestReadAuthResult_Error(t *testing.T) {
	ah := NewMySQLAuthHandler("p")
	errPkt := mysqlPacket(2, append([]byte{0xFF, 0x15, 0x04}, []byte("#28000denied")...))
	if err := ah.ReadAuthResult(bytes.NewReader(errPkt)); err == nil {
		t.Error("expected error for ERR auth result")
	}
}

package mysqlcompat

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestProtocolHandshakeAndQuerySmoke(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	deadline := time.Now().Add(2 * time.Second)
	if err := server.SetDeadline(deadline); err != nil {
		t.Fatalf("server deadline: %v", err)
	}
	if err := client.SetDeadline(deadline); err != nil {
		t.Fatalf("client deadline: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- handleConn(server)
	}()

	seq, handshake := readClientPacket(t, client)
	if seq != 0 || len(handshake) == 0 || handshake[0] != 0x0a {
		t.Fatalf("bad handshake seq=%d payload=%x", seq, handshake)
	}

	writeClientPacket(t, client, 1, handshakeResponsePayload())
	seq, ok := readClientPacket(t, client)
	if seq != 2 || len(ok) == 0 || ok[0] != 0x00 {
		t.Fatalf("bad handshake OK seq=%d payload=%x", seq, ok)
	}

	writeClientPacket(t, client, 0, append([]byte{comQuery}, []byte("SELECT VERSION()")...))
	seq, resultHeader := readClientPacket(t, client)
	if seq != 1 || len(resultHeader) != 1 || resultHeader[0] != 1 {
		t.Fatalf("bad result header seq=%d payload=%x", seq, resultHeader)
	}
	_, _ = readClientPacket(t, client) // column definition
	_, _ = readClientPacket(t, client) // EOF
	_, row := readClientPacket(t, client)
	if !bytes.Contains(row, []byte("5.7.0-TsunamiDB")) {
		t.Fatalf("row payload does not contain version: %x", row)
	}
	_, _ = readClientPacket(t, client) // EOF

	writeClientPacket(t, client, 0, []byte{comQuit})
	if err := <-done; err != nil {
		t.Fatalf("handleConn: %v", err)
	}
}

func handshakeResponsePayload() []byte {
	capability := clientLongPassword |
		clientLongFlag |
		clientConnectWithDB |
		clientProtocol41 |
		clientTransactions |
		clientSecureConnection |
		clientPluginAuth

	var payload bytes.Buffer
	_ = binary.Write(&payload, binary.LittleEndian, capability)
	_ = binary.Write(&payload, binary.LittleEndian, uint32(0))
	payload.WriteByte(33)
	payload.Write(make([]byte, 23))
	writeNullTerminated(&payload, "root")
	payload.WriteByte(0)
	writeNullTerminated(&payload, defaultDatabase)
	writeNullTerminated(&payload, "mysql_native_password")
	return payload.Bytes()
}

func readClientPacket(t *testing.T, conn net.Conn) (byte, []byte) {
	t.Helper()
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatalf("read header: %v", err)
	}
	length := int(uint32(header[0]) | uint32(header[1])<<8 | uint32(header[2])<<16)
	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	return header[3], payload
}

func writeClientPacket(t *testing.T, conn net.Conn, seq byte, payload []byte) {
	t.Helper()
	header := []byte{
		byte(len(payload)),
		byte(len(payload) >> 8),
		byte(len(payload) >> 16),
		seq,
	}
	if _, err := conn.Write(append(header, payload...)); err != nil {
		t.Fatalf("write packet: %v", err)
	}
}

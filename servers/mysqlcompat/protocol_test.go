package mysqlcompat

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/PAW122/TsunamiDB/data/relational"
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

func TestProtocolFieldListReturnsTableColumns(t *testing.T) {
	withTempWorkingDir(t)
	if _, err := relational.ExecuteSQL("CREATE TABLE client_flow (id uint64 INDEXED, name string(32), active bool, score float64)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

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

	_, _ = readClientPacket(t, client)
	writeClientPacket(t, client, 1, handshakeResponsePayload())
	_, ok := readClientPacket(t, client)
	if len(ok) == 0 || ok[0] != 0x00 {
		t.Fatalf("bad handshake OK payload=%x", ok)
	}

	writeClientPacket(t, client, 0, append([]byte{comFieldList}, []byte("client_flow\x00%")...))
	var names []string
	for {
		_, payload := readClientPacket(t, client)
		if len(payload) > 0 && payload[0] == 0xfe && len(payload) <= 5 {
			break
		}
		def := parseColumnDefinitionPacket(t, payload)
		if def.schema != defaultDatabase || def.table != "client_flow" || def.orgTable != "client_flow" {
			t.Fatalf("bad field metadata: %+v", def)
		}
		names = append(names, def.name)
	}
	if got := strings.Join(names, ","); got != "id,name,active,score" {
		t.Fatalf("field names = %q, want id,name,active,score", got)
	}

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

type columnDefinitionSnapshot struct {
	schema   string
	table    string
	orgTable string
	name     string
	orgName  string
}

func parseColumnDefinitionPacket(t *testing.T, payload []byte) columnDefinitionSnapshot {
	t.Helper()
	var out columnDefinitionSnapshot
	var ok bool
	_, payload, ok = readLenencString(payload)
	if !ok {
		t.Fatalf("parse catalog from %x", payload)
	}
	out.schema, payload, ok = readLenencString(payload)
	if !ok {
		t.Fatalf("parse schema")
	}
	out.table, payload, ok = readLenencString(payload)
	if !ok {
		t.Fatalf("parse table")
	}
	out.orgTable, payload, ok = readLenencString(payload)
	if !ok {
		t.Fatalf("parse org table")
	}
	out.name, payload, ok = readLenencString(payload)
	if !ok {
		t.Fatalf("parse name")
	}
	out.orgName, _, ok = readLenencString(payload)
	if !ok {
		t.Fatalf("parse org name")
	}
	return out
}

func readLenencString(payload []byte) (string, []byte, bool) {
	n, read := readLengthEncodedInteger(payload)
	if read == 0 || uint64(len(payload)-read) < n {
		return "", nil, false
	}
	start := read
	end := start + int(n)
	return string(payload[start:end]), payload[end:], true
}

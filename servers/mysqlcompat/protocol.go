package mysqlcompat

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
)

var nextConnectionID uint32 = 100

type session struct {
	packets *packetConn
	db      string
}

// Run starts a MySQL-compatible TCP endpoint on the given port.
func Run(port int) error {
	return ListenAndServe(":" + strconv.Itoa(port))
}

// ListenAndServe starts a MySQL-compatible TCP endpoint on addr.
func ListenAndServe(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go func() {
			if err := handleConn(conn); err != nil && !errors.Is(err, net.ErrClosed) {
				log.Println("mysql compatibility connection stopped:", err)
			}
		}()
	}
}

func handleConn(conn net.Conn) error {
	defer conn.Close()

	s := &session{
		packets: newPacketConn(conn),
		db:      defaultDatabase,
	}
	if err := s.handshake(); err != nil {
		return err
	}
	return s.commandLoop()
}

func (s *session) handshake() error {
	salt := make([]byte, 20)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	for i := range salt {
		if salt[i] == 0 {
			salt[i] = byte(i + 1)
		}
	}

	capability := clientLongPassword |
		clientFoundRows |
		clientLongFlag |
		clientConnectWithDB |
		clientProtocol41 |
		clientTransactions |
		clientSecureConnection |
		clientPluginAuth |
		clientPluginAuthLenencClientData |
		clientConnectAttrs
	s.packets.capability = capability

	connID := atomic.AddUint32(&nextConnectionID, 1)
	var payload bytes.Buffer
	payload.WriteByte(0x0a)
	writeNullTerminated(&payload, "5.7.0-TsunamiDB")
	_ = binary.Write(&payload, binary.LittleEndian, connID)
	payload.Write(salt[:8])
	payload.WriteByte(0)
	_ = binary.Write(&payload, binary.LittleEndian, uint16(capability))
	payload.WriteByte(33)
	_ = binary.Write(&payload, binary.LittleEndian, uint16(serverStatusAutocommit))
	_ = binary.Write(&payload, binary.LittleEndian, uint16(capability>>16))
	payload.WriteByte(21)
	payload.Write(make([]byte, 10))
	payload.Write(salt[8:])
	payload.WriteByte(0)
	writeNullTerminated(&payload, "mysql_native_password")

	if err := s.packets.writePacket(payload.Bytes()); err != nil {
		return err
	}

	response, err := s.packets.readPacket()
	if err != nil {
		return err
	}
	s.applyHandshakeResponse(response)
	return s.writeOK(0, 0)
}

func (s *session) applyHandshakeResponse(payload []byte) {
	if len(payload) < 36 {
		return
	}
	clientCapability := binary.LittleEndian.Uint32(payload[:4])
	s.packets.capability = clientCapability
	pos := 32
	if next := bytes.IndexByte(payload[pos:], 0); next >= 0 {
		pos += next + 1
	} else {
		return
	}

	switch {
	case clientCapability&clientPluginAuthLenencClientData != 0:
		_, read := readLengthEncodedInteger(payload[pos:])
		pos += read
	case clientCapability&clientSecureConnection != 0 && pos < len(payload):
		authLen := int(payload[pos])
		pos += 1 + authLen
	default:
		if next := bytes.IndexByte(payload[pos:], 0); next >= 0 {
			pos += next + 1
		}
	}

	if clientCapability&clientConnectWithDB != 0 && pos < len(payload) {
		if next := bytes.IndexByte(payload[pos:], 0); next >= 0 {
			db := string(payload[pos : pos+next])
			if db != "" {
				s.db = db
			}
		}
	}
}

func readLengthEncodedInteger(payload []byte) (uint64, int) {
	if len(payload) == 0 {
		return 0, 0
	}
	switch payload[0] {
	case 0xfc:
		if len(payload) < 3 {
			return 0, len(payload)
		}
		return uint64(binary.LittleEndian.Uint16(payload[1:3])), 3
	case 0xfd:
		if len(payload) < 4 {
			return 0, len(payload)
		}
		return uint64(payload[1]) | uint64(payload[2])<<8 | uint64(payload[3])<<16, 4
	case 0xfe:
		if len(payload) < 9 {
			return 0, len(payload)
		}
		return binary.LittleEndian.Uint64(payload[1:9]), 9
	default:
		return uint64(payload[0]), 1
	}
}

func (s *session) commandLoop() error {
	for {
		payload, err := s.packets.readPacket()
		if err != nil {
			return err
		}
		if len(payload) == 0 {
			if err := s.writeError(1047, "08S01", "empty command"); err != nil {
				return err
			}
			continue
		}

		s.packets.startCommandResponse()
		switch payload[0] {
		case comQuit:
			return nil
		case comPing:
			if err := s.writeOK(0, 0); err != nil {
				return err
			}
		case comInitDB:
			db := strings.TrimSpace(string(payload[1:]))
			if db == "" {
				db = defaultDatabase
			}
			s.db = db
			if err := s.writeOK(0, 0); err != nil {
				return err
			}
		case comQuery:
			if err := s.handleQuery(string(payload[1:])); err != nil {
				return err
			}
		case comFieldList:
			if err := s.writeEOF(); err != nil {
				return err
			}
		default:
			if err := s.writeError(1047, "08S01", fmt.Sprintf("unsupported command 0x%x", payload[0])); err != nil {
				return err
			}
		}
	}
}

func (s *session) handleQuery(query string) error {
	result, err := executeCompatQuery(s.db, query)
	if err != nil {
		return s.writeError(1064, "42000", err.Error())
	}
	if result.columns == nil {
		return s.writeOK(result.affectedRows, result.insertID)
	}
	return s.writeResultset(result)
}

func (s *session) writeOK(affectedRows, insertID uint64) error {
	var payload bytes.Buffer
	payload.WriteByte(0x00)
	writeLengthEncodedInteger(&payload, affectedRows)
	writeLengthEncodedInteger(&payload, insertID)
	_ = binary.Write(&payload, binary.LittleEndian, serverStatusAutocommit)
	_ = binary.Write(&payload, binary.LittleEndian, uint16(0))
	return s.packets.writePacket(payload.Bytes())
}

func (s *session) writeError(code uint16, state, message string) error {
	var payload bytes.Buffer
	payload.WriteByte(0xff)
	_ = binary.Write(&payload, binary.LittleEndian, code)
	payload.WriteByte('#')
	if len(state) != 5 {
		state = "HY000"
	}
	payload.WriteString(state)
	payload.WriteString(message)
	return s.packets.writePacket(payload.Bytes())
}

func (s *session) writeEOF() error {
	var payload bytes.Buffer
	payload.WriteByte(0xfe)
	_ = binary.Write(&payload, binary.LittleEndian, uint16(0))
	_ = binary.Write(&payload, binary.LittleEndian, serverStatusAutocommit)
	return s.packets.writePacket(payload.Bytes())
}

func (s *session) writeResultset(result *queryResult) error {
	var payload bytes.Buffer
	writeLengthEncodedInteger(&payload, uint64(len(result.columns)))
	if err := s.packets.writePacket(payload.Bytes()); err != nil {
		return err
	}

	for _, col := range result.columns {
		if err := s.writeColumn(col); err != nil {
			return err
		}
	}
	if err := s.writeEOF(); err != nil {
		return err
	}

	for _, row := range result.rows {
		payload.Reset()
		for i := range result.columns {
			var value any
			if i < len(row) {
				value = row[i]
			}
			writeLengthEncodedValue(&payload, value)
		}
		if err := s.packets.writePacket(payload.Bytes()); err != nil {
			return err
		}
	}
	return s.writeEOF()
}

func (s *session) writeColumn(col column) error {
	var payload bytes.Buffer
	writeLengthEncodedString(&payload, "def")
	writeLengthEncodedString(&payload, s.db)
	writeLengthEncodedString(&payload, "")
	writeLengthEncodedString(&payload, "")
	writeLengthEncodedString(&payload, col.name)
	writeLengthEncodedString(&payload, col.name)
	payload.WriteByte(0x0c)
	_ = binary.Write(&payload, binary.LittleEndian, uint16(33))
	if col.length == 0 {
		col.length = 1024
	}
	_ = binary.Write(&payload, binary.LittleEndian, col.length)
	payload.WriteByte(col.typ)
	_ = binary.Write(&payload, binary.LittleEndian, col.flags)
	payload.WriteByte(col.decimals)
	payload.Write([]byte{0, 0})
	return s.packets.writePacket(payload.Bytes())
}

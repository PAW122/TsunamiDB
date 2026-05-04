package mysqlcompat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"time"
)

type packetConn struct {
	conn       net.Conn
	capability uint32
	sequenceID byte
}

func newPacketConn(conn net.Conn) *packetConn {
	return &packetConn{conn: conn}
}

func (c *packetConn) readPacket() ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return nil, err
	}
	length := int(uint32(header[0]) | uint32(header[1])<<8 | uint32(header[2])<<16)
	c.sequenceID = header[3] + 1
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.conn, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *packetConn) writePacket(payload []byte) error {
	if len(payload) > math.MaxUint32>>8 {
		return fmt.Errorf("mysqlcompat: packet too large")
	}
	header := []byte{
		byte(len(payload)),
		byte(len(payload) >> 8),
		byte(len(payload) >> 16),
		c.sequenceID,
	}
	c.sequenceID++
	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	_, err := c.conn.Write(payload)
	return err
}

func (c *packetConn) startCommandResponse() {
	c.sequenceID = 1
}

func writeNullTerminated(buf *bytes.Buffer, value string) {
	buf.WriteString(value)
	buf.WriteByte(0)
}

func writeLengthEncodedInteger(buf *bytes.Buffer, value uint64) {
	switch {
	case value < 251:
		buf.WriteByte(byte(value))
	case value <= math.MaxUint16:
		buf.WriteByte(0xfc)
		_ = binary.Write(buf, binary.LittleEndian, uint16(value))
	case value <= 0xffffff:
		buf.WriteByte(0xfd)
		buf.WriteByte(byte(value))
		buf.WriteByte(byte(value >> 8))
		buf.WriteByte(byte(value >> 16))
	default:
		buf.WriteByte(0xfe)
		_ = binary.Write(buf, binary.LittleEndian, value)
	}
}

func writeLengthEncodedString(buf *bytes.Buffer, value string) {
	writeLengthEncodedInteger(buf, uint64(len(value)))
	buf.WriteString(value)
}

func writeLengthEncodedValue(buf *bytes.Buffer, value any) {
	if value == nil {
		buf.WriteByte(0xfb)
		return
	}
	writeLengthEncodedString(buf, mysqlTextValue(value))
}

func mysqlTextValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case bool:
		if v {
			return "1"
		}
		return "0"
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprint(value)
	}
}

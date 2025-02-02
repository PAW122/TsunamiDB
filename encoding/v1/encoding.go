package encoding_v1

import (
	"TsunamiDB/types"
	"bytes"
	"encoding/binary"
)

func Encode(data string) ([]byte, types.Encoded) {
	var buf bytes.Buffer

	// Wersja (2 bajty) - przykładowa wersja 1.0 (0x01 0x00)
	binary.Write(&buf, binary.LittleEndian, uint16(1))

	// Miejsce na wskaźniki (wypełniane później)
	startPos := buf.Len() + 4 + 4 + 4 // 2 bajty wersji + 4 na start + 4 na end + 4 na długość
	endPos := startPos + len(data)

	// DataStart Pointer (4 bajty)
	binary.Write(&buf, binary.LittleEndian, uint32(startPos))

	// DataEnd Pointer (4 bajty)
	binary.Write(&buf, binary.LittleEndian, uint32(endPos))

	// DataLength (4 bajty)
	binary.Write(&buf, binary.LittleEndian, uint32(len(data)))

	// Data (treść)
	buf.WriteString(data)

	res_data := types.Encoded{
		Version:      1,
		StartPointer: startPos,
		EndPointer:   endPos,
	}

	return buf.Bytes(), res_data
}

/*
dane:
2 bytes -> version
dataStart Pointer
dataEnd Pointer
dataLength
data
*/

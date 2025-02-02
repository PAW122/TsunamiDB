package encoding_v1

import (
	"TsunamiDB/types"
	"bytes"
	"encoding/binary"
)

// Encode encodes a string into a custom binary format
func Encode(data string) ([]byte, types.Encoded) {
	var buf bytes.Buffer

	// Wersja (2 bajty) - przykładowa wersja 1.0 (0x01 0x00)
	binary.Write(&buf, binary.LittleEndian, uint16(1))

	// StartPointer powinien wskazywać na początek DANYCH, więc pomijamy nagłówek
	headerSize := 2 + 4 + 4 + 4 // version(2) + startPtr(4) + endPtr(4) + length(4)
	startPtr := headerSize
	endPtr := startPtr + len(data)

	// DataStart Pointer (4 bajty)
	binary.Write(&buf, binary.LittleEndian, uint32(startPtr))

	// DataEnd Pointer (4 bajty)
	binary.Write(&buf, binary.LittleEndian, uint32(endPtr))

	// DataLength (4 bajty)
	binary.Write(&buf, binary.LittleEndian, uint32(len(data)))

	// Właściwe dane
	buf.WriteString(data)

	// Struktura wynikowa
	res_data := types.Encoded{
		Version:      1,
		StartPointer: startPtr,
		EndPointer:   endPtr,
	}

	return buf.Bytes(), res_data
}

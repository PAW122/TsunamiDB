package encoding_v1

import (
	"TsunamiDB/types"
	"bytes"
	"encoding/binary"
)

// Encode encodes a byte slice into a custom binary format
func Encode(data []byte) ([]byte, types.Encoded) {
	var buf bytes.Buffer

	// Wersja (2 bajty)
	binary.Write(&buf, binary.LittleEndian, uint16(1))

	// Określenie rozmiaru pointera
	headerSize := 2 + 1 + 4 // version(2) + pointerSize(1) + dataLength(4)
	startPtr := headerSize
	endPtr := startPtr + len(data)

	// Określenie najmniejszego możliwego rozmiaru pointera
	var pointerSize uint8
	if endPtr < 256 {
		pointerSize = 1 // uint8
	} else if endPtr < 65536 {
		pointerSize = 2 // uint16
	} else if endPtr < 4294967296 {
		pointerSize = 4 // uint32
	} else {
		pointerSize = 8 // uint64
	}

	// Zapisz wielkość wskaźnika
	binary.Write(&buf, binary.LittleEndian, pointerSize)

	// Zapisz startPtr i endPtr w odpowiednim formacie
	switch pointerSize {
	case 1:
		binary.Write(&buf, binary.LittleEndian, uint8(startPtr))
		binary.Write(&buf, binary.LittleEndian, uint8(endPtr))
	case 2:
		binary.Write(&buf, binary.LittleEndian, uint16(startPtr))
		binary.Write(&buf, binary.LittleEndian, uint16(endPtr))
	case 4:
		binary.Write(&buf, binary.LittleEndian, uint32(startPtr))
		binary.Write(&buf, binary.LittleEndian, uint32(endPtr))
	case 8:
		binary.Write(&buf, binary.LittleEndian, uint64(startPtr))
		binary.Write(&buf, binary.LittleEndian, uint64(endPtr))
	}

	// Zapisz długość danych (4 bajty)
	binary.Write(&buf, binary.LittleEndian, uint32(len(data)))

	// Zapisz dane
	buf.Write(data)

	// Struktura wynikowa
	res_data := types.Encoded{
		Version:      1,
		StartPointer: startPtr,
		EndPointer:   endPtr,
	}

	return buf.Bytes(), res_data
}

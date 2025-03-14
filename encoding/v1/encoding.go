package encoding_v1

import (
	"bytes"
	"encoding/binary"

	"github.com/PAW122/TsunamiDB/types"

	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

// Encode encodes a byte slice into a custom binary format
func Encode(data []byte) ([]byte, types.Encoded) {
	defer debug.MeasureTime("encode")()

	var buf bytes.Buffer

	// Version (1 bajt)
	binary.Write(&buf, binary.LittleEndian, uint8(1))

	// Określenie rozmiaru pointera
	headerSize := 1 + 1 // version(1) + pointerSize(1)
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

	// pointerSize (1)
	binary.Write(&buf, binary.LittleEndian, pointerSize)

	// Zapisz startPtr i endPtr w odpowiednim formacie
	/*
		startPtr (8)
		endPtr(8 - 64)
	*/
	switch pointerSize {
	case 1:
		binary.Write(&buf, binary.LittleEndian, uint8(startPtr))
		binary.Write(&buf, binary.LittleEndian, uint8(endPtr))
	case 2:
		binary.Write(&buf, binary.LittleEndian, uint8(startPtr))
		binary.Write(&buf, binary.LittleEndian, uint16(endPtr))
	case 4:
		binary.Write(&buf, binary.LittleEndian, uint8(startPtr))
		binary.Write(&buf, binary.LittleEndian, uint32(endPtr))
	case 8:
		binary.Write(&buf, binary.LittleEndian, uint8(startPtr))
		binary.Write(&buf, binary.LittleEndian, uint64(endPtr))
	}

	// Zapisz długość danych (4 bajty) - nie potrzebne, latwe do obl z ptr
	// binary.Write(&buf, binary.LittleEndian, uint32(len(data)))

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

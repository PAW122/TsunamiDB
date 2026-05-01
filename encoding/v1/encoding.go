package encoding_v1

import (
	"bytes"
	"encoding/binary"

	"github.com/PAW122/TsunamiDB/types"

	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

// Encode encodes a byte slice into a custom binary format
func Encode(data []byte, hasNested bool) ([]byte, types.Encoded) {
	defer debug.MeasureTime("encode")()

	var buf bytes.Buffer

	// Version (1 bajt)
	binary.Write(&buf, binary.LittleEndian, uint8(1))

	// Określenie rozmiaru pointera
	headerSize := 1 + 1 // version(1) + pointerSize(1)
	startPtr := headerSize
	endPtr := startPtr + len(data)

	// Określenie najmniejszego możliwego rozmiaru pointera
	pointerSize := pointerSizeForEnd(endPtr)

	// pointerSize (1) + nested flag (bit7)
	pointerHeader := pointerSize
	if hasNested {
		pointerHeader |= 0x80
	}
	binary.Write(&buf, binary.LittleEndian, pointerHeader)

	// Zapisz startPtr i endPtr w odpowiednim formacie
	/*
		startPtr (8)
		endPtr(8 - 64)
	*/
	writeEncodedPointers(&buf, pointerSize, startPtr, endPtr)

	// Zapisz długość danych (4 bajty) - nie potrzebne, latwe do obl z ptr
	// binary.Write(&buf, binary.LittleEndian, uint32(len(data)))

	// Zapisz dane
	buf.Write(data)

	// Struktura wynikowa
	res_data := types.Encoded{
		Version:      1,
		StartPointer: startPtr,
		EndPointer:   endPtr,
		HasNested:    hasNested,
	}

	return buf.Bytes(), res_data
}

func pointerSizeForEnd(endPtr int) uint8 {
	if endPtr < 256 {
		return 1 // uint8
	}
	if endPtr < 65536 {
		return 2 // uint16
	}
	if int64(endPtr) < int64(4294967296) {
		return 4 // uint32
	}
	return 8 // uint64
}

func writeEncodedPointers(buf *bytes.Buffer, pointerSize uint8, startPtr, endPtr int) {
	switch pointerSize {
	case 1:
		binary.Write(buf, binary.LittleEndian, uint8(startPtr))
		binary.Write(buf, binary.LittleEndian, uint8(endPtr))
	case 2:
		binary.Write(buf, binary.LittleEndian, uint8(startPtr))
		binary.Write(buf, binary.LittleEndian, uint16(endPtr))
	case 4:
		binary.Write(buf, binary.LittleEndian, uint8(startPtr))
		binary.Write(buf, binary.LittleEndian, uint32(endPtr))
	case 8:
		binary.Write(buf, binary.LittleEndian, uint8(startPtr))
		binary.Write(buf, binary.LittleEndian, uint64(endPtr))
	}
}

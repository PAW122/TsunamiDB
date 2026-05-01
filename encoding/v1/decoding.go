package encoding_v1

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/PAW122/TsunamiDB/types"

	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

func DecodeRawData(data []byte) string {
	fmt.Println("Decoding raw data:", data)
	return string(data) // Po prostu zwracamy stringa, bo dane to surowa sekcja `data`
}

// Decode poprawnie odczytuje dane binarne
func Decode(data []byte) types.Decoded {
	defer debug.MeasureTime("decode")()

	var decoded types.Decoded
	buf := bytes.NewReader(data)

	// Odczytanie wersji (1 bajt)
	var version uint8
	binary.Read(buf, binary.LittleEndian, &version)
	decoded.Version = int(version)

	// Odczytanie wielkości wskaźnika (1 bajt)
	var pointerHeader uint8
	binary.Read(buf, binary.LittleEndian, &pointerHeader)
	hasNested := (pointerHeader & 0x80) != 0
	pointerSize := pointerHeader & 0x7F
	decoded.HasNested = hasNested

	// Odczytanie StartPointer i EndPointer zgodnie z rozmiarem
	var startPos, endPos uint64
	switch pointerSize {
	case 1:
		var tempStart, tempEnd uint8
		binary.Read(buf, binary.LittleEndian, &tempStart)
		binary.Read(buf, binary.LittleEndian, &tempEnd)
		startPos, endPos = uint64(tempStart), uint64(tempEnd)
	case 2:
		var (
			tempStart uint8
			tempEnd   uint16
		)
		binary.Read(buf, binary.LittleEndian, &tempStart)
		binary.Read(buf, binary.LittleEndian, &tempEnd)
		startPos, endPos = uint64(tempStart), uint64(tempEnd)
	case 4:
		var (
			tempStart uint8
			tempEnd   uint32
		)
		binary.Read(buf, binary.LittleEndian, &tempStart)
		binary.Read(buf, binary.LittleEndian, &tempEnd)
		startPos, endPos = uint64(tempStart), uint64(tempEnd)
	case 8:
		var tempStart uint8
		binary.Read(buf, binary.LittleEndian, &tempStart)
		binary.Read(buf, binary.LittleEndian, &endPos)
		startPos = uint64(tempStart)
	default:
		fmt.Println("Invalid pointer size:", pointerSize)
		debug.LogExtra("Pointer size: ", pointerSize)
		debug.LogExtra("Data:", data)
		return decoded
	}

	decoded.StartPointer = int(startPos)
	decoded.EndPointer = int(endPos)
	if endPos < startPos {
		debug.LogExtra("Invalid pointer range:", startPos, endPos)
		return decoded
	}

	// Odczytanie długości danych (4 bajty) (metadata)
	// var dataLen uint32
	// binary.Read(buf, binary.LittleEndian, &dataLen)
	// decoded.Length = int(dataLen))

	// Odczytanie właściwych danych (Data)
	dataLen := endPos - startPos
	if dataLen > uint64(buf.Len()) {
		debug.LogExtra("Invalid data length:", dataLen, "remaining:", buf.Len())
		return decoded
	}
	decodedData := make([]byte, dataLen)
	buf.Read(decodedData)
	decoded.Data = string(decodedData)

	return decoded
}

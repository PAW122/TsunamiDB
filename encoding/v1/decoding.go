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
	var pointerSize uint8
	binary.Read(buf, binary.LittleEndian, &pointerSize)

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
	default:
		fmt.Println("Invalid pointer size:", pointerSize)
		return decoded
	}

	decoded.StartPointer = int(startPos)
	decoded.EndPointer = int(endPos)

	// Odczytanie długości danych (4 bajty) (metadata)
	// var dataLen uint32
	// binary.Read(buf, binary.LittleEndian, &dataLen)
	// decoded.Length = int(dataLen))

	// Odczytanie właściwych danych (Data)
	dataLen := endPos - startPos
	decodedData := make([]byte, dataLen)
	buf.Read(decodedData)
	decoded.Data = string(decodedData)

	return decoded
}

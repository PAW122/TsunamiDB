package encoding_v1

import (
	"TsunamiDB/types"
	"bytes"
	"encoding/binary"
)

func DecodeRawData(data []byte) string {
	return string(data) // Po prostu zwracamy stringa, bo dane to surowa sekcja `data`
}

// Decode poprawnie odczytuje dane binarne
func Decode(data []byte) types.Decoded {
	var decoded types.Decoded
	buf := bytes.NewReader(data)

	// Odczytanie wersji (2 bajty)
	var version uint16
	binary.Read(buf, binary.LittleEndian, &version)
	decoded.Version = int(version)

	// Odczytanie StartPointer (4 bajty)
	var startPos uint32
	binary.Read(buf, binary.LittleEndian, &startPos)
	decoded.StartPointer = int(startPos)

	// Odczytanie EndPointer (4 bajty)
	var endPos uint32
	binary.Read(buf, binary.LittleEndian, &endPos)
	decoded.EndPointer = int(endPos)

	// Odczytanie długości danych (4 bajty)
	var dataLen uint32
	binary.Read(buf, binary.LittleEndian, &dataLen)
	decoded.Length = int(dataLen)

	// Odczytanie właściwych danych
	decodedData := make([]byte, dataLen)
	buf.Read(decodedData)
	decoded.Data = string(decodedData)

	return decoded
}

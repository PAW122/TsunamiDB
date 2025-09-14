package encoding_v1

import (
	"encoding/binary"
	"errors"

	"github.com/PAW122/TsunamiDB/types"
)

/*
Layout (total = entrySize + 3 bytes):

byte 0:
  bit0 -> skipBit (1=skip/DELETED)
  bit1 -> nextEntryPointerBit (valid only if skipBit==1)
  bit2..bit7 -> 0

bytes 1 .. total-1:
  if skipBit == 1:
     [1..8]   -> optional uint64 nextEntryPointer (LE) when nextEntryPointerBit==1
     [9..end] -> zeros (no dataStart marker, no data)
  if skipBit == 0:
     [1..pos-1] -> zeros (padding)
      pos       -> dataStartBit = 0x01
     [pos+1..]  -> payload bytes (len<=entrySize)

pos = total - len(payload) - 1
*/

// EncodeIncEntry koduje pojedynczy wpis inc-table do stałej długości (entrySize+3).
// Jeśli body ma długość 0..entrySize -> skipBit=0, marker 0x01 przed danymi.
// Jeśli chcesz trwale „usunąć” (skip), wywołaj z body=nil oraz skip=true (patrz SetSkipIncEntry).
func EncodeIncEntry(entrySize uint64, body []byte) []byte {
	total := int(entrySize) + 3
	buf := make([]byte, total)

	// Normalny wpis (skipBit=0): dane muszą zmieścić się w entrySize
	if uint64(len(body)) > entrySize {
		// Nie mieszczą się: zwróć pusty bufor (caller powinien walidować wcześniej).
		return nil
	}

	// Ustawiamy skipBit=0 (domyślnie)
	// nextEntryPointerBit=0 (domyślnie)
	// padding = zero

	// Wstaw marker 0x01 i dane na końcu
	pos := total - len(body) - 1
	if pos < 1 {
		// Teoretycznie nie powinno się zdarzyć (mamy +3 bajty na overhead)
		return nil
	}
	buf[pos] = 0x01
	copy(buf[pos+1:], body)
	return buf
}

// SetSkipIncEntry ustawia wpis jako „skip” (usunięty).
// Jeżeli chcesz wskazać skok do następnego „nieskipowanego” wpisu, podaj nextEntryPointer>0.
// Funkcja modyfikuje bufor zwrócony przez EncodeIncEntry (albo przygotowany zera/placeholder).
func SetSkipIncEntry(entry []byte, nextEntryPointer uint64) {
	if len(entry) == 0 {
		return
	}
	// bit0 = skipBit
	entry[0] |= 0b0000_0001

	if nextEntryPointer > 0 {
		// bit1 = nextEntryPointerBit
		entry[0] |= 0b0000_0010
		// zapisz uint64 LE w bytes [1..8]
		if len(entry) >= 9 {
			binary.LittleEndian.PutUint64(entry[1:9], nextEntryPointer)
		}
	}
	// Reszta zostaje zerowa; nie ustawiamy żadnego dataStartBit w trybie skip.
}

// DecodeIncEntry dekoduje wpis inc-table do struktury IncTableBody.
// Gdy SkipBit==true, Data jest puste; NextEntryPointer wypełniony jeśli obecny.
// Gdy SkipBit==false, Data to bajty za znacznikiem 0x01.
func DecodeIncEntry(entrySize uint64, raw []byte) (types.IncTableBody, error) {
	total := int(entrySize) + 3
	if len(raw) != total {
		return types.IncTableBody{}, errors.New("invalid entry length")
	}

	first := raw[0]
	skip := (first & 0b0000_0001) != 0
	nextPtrBit := (first & 0b0000_0010) != 0

	if skip {
		var ptr uint64
		if nextPtrBit && len(raw) >= 9 {
			ptr = binary.LittleEndian.Uint64(raw[1:9])
		}
		return types.IncTableBody{
			Data:             nil,
			EntrySize:        entrySize,
			SkipBit:          true,
			NextEntryPointer: ptr,
		}, nil
	}

	// Szukamy markera 0x01 (dataStartBit)
	// Skanujemy od 1 (byte0 to bity sterujące)
	pos := -1
	for i := 1; i < total; i++ {
		if raw[i] == 0x01 {
			pos = i
			break
		}
	}
	if pos == -1 {
		// Brak markera — traktuj jako pusty payload
		return types.IncTableBody{
			Data:             nil,
			EntrySize:        entrySize,
			SkipBit:          false,
			NextEntryPointer: 0,
		}, nil
	}

	data := raw[pos+1:]
	return types.IncTableBody{
		Data:             data,
		EntrySize:        entrySize,
		SkipBit:          false,
		NextEntryPointer: 0,
	}, nil
}

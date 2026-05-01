package encoding_v1

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

type errReader struct {
	err error
}

func (r errReader) Read([]byte) (int, error) {
	return 0, r.err
}

func resetEncryptionHooks(t *testing.T) {
	t.Helper()
	newCipher = aes.NewCipher
	newGCM = cipher.NewGCM
	randReader = cryptoRand.Reader
	t.Cleanup(func() {
		newCipher = aes.NewCipher
		newGCM = cipher.NewGCM
		randReader = cryptoRand.Reader
	})
}

func TestEncodeDecodeRoundTripAndPointerSizes(t *testing.T) {
	tests := []struct {
		name        string
		dataLen     int
		hasNested   bool
		pointerSize byte
	}{
		{name: "uint8 pointer", dataLen: 3, hasNested: false, pointerSize: 1},
		{name: "uint16 pointer", dataLen: 254, hasNested: true, pointerSize: 2},
		{name: "uint32 pointer", dataLen: 65534, hasNested: false, pointerSize: 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := bytes.Repeat([]byte("x"), tc.dataLen)
			encoded, meta := Encode(data, tc.hasNested)

			if meta.Version != 1 || meta.StartPointer != 2 || meta.EndPointer != 2+tc.dataLen || meta.HasNested != tc.hasNested {
				t.Fatalf("unexpected metadata: %#v", meta)
			}
			if got := encoded[1] & 0x7F; got != tc.pointerSize {
				t.Fatalf("pointer size header = %d, want %d", got, tc.pointerSize)
			}
			if got := (encoded[1] & 0x80) != 0; got != tc.hasNested {
				t.Fatalf("nested bit = %v, want %v", got, tc.hasNested)
			}

			decoded := Decode(encoded)
			if decoded.Version != 1 || decoded.StartPointer != 2 || decoded.EndPointer != 2+tc.dataLen || decoded.HasNested != tc.hasNested {
				t.Fatalf("unexpected decoded metadata: %#v", decoded)
			}
			if decoded.Data != string(data) {
				t.Fatalf("decoded data length = %d, want %d", len(decoded.Data), len(data))
			}
		})
	}
}

func TestPointerSizeBoundariesAndUint64PointerWrite(t *testing.T) {
	cases := map[int]uint8{
		255:                        1,
		256:                        2,
		65535:                      2,
		65536:                      4,
		int((uint64(1) << 32) - 1): 4,
		int(uint64(1) << 32):       8,
	}
	for endPtr, want := range cases {
		if got := pointerSizeForEnd(endPtr); got != want {
			t.Fatalf("pointerSizeForEnd(%d) = %d, want %d", endPtr, got, want)
		}
	}

	var buf bytes.Buffer
	endPtr := int(uint64(1) << 32)
	writeEncodedPointers(&buf, 8, 2, endPtr)

	got := buf.Bytes()
	if len(got) != 9 {
		t.Fatalf("uint64 pointer bytes length = %d, want 9", len(got))
	}
	if got[0] != 2 {
		t.Fatalf("start pointer byte = %d, want 2", got[0])
	}
	if decodedEnd := binary.LittleEndian.Uint64(got[1:9]); decodedEnd != uint64(endPtr) {
		t.Fatalf("encoded end pointer = %d, want %d", decodedEnd, endPtr)
	}
}

func TestDecodeRawDataAndMalformedPointerHeader(t *testing.T) {
	if got := DecodeRawData([]byte("raw")); got != "raw" {
		t.Fatalf("DecodeRawData returned %q", got)
	}

	decoded := Decode([]byte{1, 3})
	if decoded.Version != 1 || decoded.Data != "" || decoded.StartPointer != 0 || decoded.EndPointer != 0 {
		t.Fatalf("invalid pointer header decoded as %#v", decoded)
	}
}

func TestDecodeInvalidPointerRangeAndTruncatedPayload(t *testing.T) {
	decoded := Decode([]byte{1, 1, 4, 2})
	if decoded.StartPointer != 4 || decoded.EndPointer != 2 || decoded.Data != "" {
		t.Fatalf("invalid pointer range decoded as %#v", decoded)
	}

	decoded = Decode([]byte{1, 1, 2, 5, 'x'})
	if decoded.StartPointer != 2 || decoded.EndPointer != 5 || decoded.Data != "" {
		t.Fatalf("truncated payload decoded as %#v", decoded)
	}
}

func TestDecodeUint64Pointer(t *testing.T) {
	payload := []byte("wide")
	var raw bytes.Buffer
	if err := binary.Write(&raw, binary.LittleEndian, uint8(1)); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(&raw, binary.LittleEndian, uint8(8|0x80)); err != nil {
		t.Fatal(err)
	}
	writeEncodedPointers(&raw, 8, 2, 2+len(payload))
	raw.Write(payload)

	decoded := Decode(raw.Bytes())
	if decoded.Version != 1 || !decoded.HasNested || decoded.StartPointer != 2 || decoded.EndPointer != 6 || decoded.Data != "wide" {
		t.Fatalf("unexpected uint64 decode: %#v", decoded)
	}
}

func TestIncEntryEncodeDecode(t *testing.T) {
	entry := EncodeIncEntry(5, []byte("abc"))
	if len(entry) != 8 {
		t.Fatalf("entry length = %d, want 8", len(entry))
	}
	if entry[0] != 0 || entry[4] != 0x01 || !bytes.Equal(entry[5:], []byte("abc")) {
		t.Fatalf("unexpected encoded entry: %v", entry)
	}

	decoded, err := DecodeIncEntry(5, entry)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SkipBit || decoded.EntrySize != 5 || decoded.NextEntryPointer != 0 || !bytes.Equal(decoded.Data, []byte("abc")) {
		t.Fatalf("unexpected decoded entry: %#v", decoded)
	}

	empty := EncodeIncEntry(0, nil)
	if len(empty) != 3 || empty[2] != 0x01 {
		t.Fatalf("unexpected empty entry: %v", empty)
	}
	decoded, err = DecodeIncEntry(0, empty)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SkipBit || len(decoded.Data) != 0 {
		t.Fatalf("unexpected decoded empty entry: %#v", decoded)
	}
}

func TestIncEntryValidationAndMissingMarker(t *testing.T) {
	if got := EncodeIncEntry(2, []byte("abc")); got != nil {
		t.Fatalf("oversized body encoded as %v, want nil", got)
	}

	tooLarge := uint64(int(^uint(0)>>1)) - 2
	if got := EncodeIncEntry(tooLarge, nil); got != nil {
		t.Fatalf("too large entry size encoded as %v, want nil", got)
	}
	if _, err := DecodeIncEntry(tooLarge, nil); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("DecodeIncEntry too large error = %v", err)
	}
	if _, err := DecodeIncEntry(2, []byte{0}); err == nil || err.Error() != "invalid entry length" {
		t.Fatalf("DecodeIncEntry invalid length error = %v", err)
	}

	decoded, err := DecodeIncEntry(2, []byte{0, 0, 0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SkipBit || decoded.Data != nil || decoded.EntrySize != 2 || decoded.NextEntryPointer != 0 {
		t.Fatalf("missing marker decoded as %#v", decoded)
	}
}

func TestIncEntrySkip(t *testing.T) {
	SetSkipIncEntry(nil, 10)

	short := []byte{0}
	SetSkipIncEntry(short, 10)
	if short[0] != 0b0000_0011 {
		t.Fatalf("short skip header = %08b, want skip and next bits", short[0])
	}

	entry := EncodeIncEntry(8, []byte("data"))
	SetSkipIncEntry(entry, 12345)

	decoded, err := DecodeIncEntry(8, entry)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.SkipBit || decoded.Data != nil || decoded.EntrySize != 8 || decoded.NextEntryPointer != 12345 {
		t.Fatalf("unexpected skip entry: %#v", decoded)
	}

	entry = EncodeIncEntry(1, []byte("x"))
	SetSkipIncEntry(entry, 0)
	decoded, err = DecodeIncEntry(1, entry)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.SkipBit || decoded.NextEntryPointer != 0 {
		t.Fatalf("unexpected skip entry without next pointer: %#v", decoded)
	}
}

func TestEncryptDecryptRoundTripAndDeriveKey(t *testing.T) {
	resetEncryptionHooks(t)

	key, err := deriveKey("abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 || string(key[:6]) != "abcabc" {
		t.Fatalf("unexpected derived key: %q", key)
	}

	plaintext := []byte("payload")
	ciphertext, err := Encrypt(plaintext, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext should not equal plaintext")
	}

	got, err := Decrypt(ciphertext, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypt = %q, want %q", got, plaintext)
	}
}

func TestEncryptErrors(t *testing.T) {
	t.Run("empty key", func(t *testing.T) {
		resetEncryptionHooks(t)
		if _, err := Encrypt([]byte("data"), ""); err == nil || err.Error() != "key cannot be empty" {
			t.Fatalf("Encrypt empty key error = %v", err)
		}
	})

	t.Run("cipher creation", func(t *testing.T) {
		resetEncryptionHooks(t)
		sentinel := errors.New("cipher failed")
		newCipher = func([]byte) (cipher.Block, error) {
			return nil, sentinel
		}
		if _, err := Encrypt([]byte("data"), "key"); err == nil || !strings.Contains(err.Error(), "error creating cipher") {
			t.Fatalf("Encrypt cipher error = %v", err)
		}
	})

	t.Run("gcm creation", func(t *testing.T) {
		resetEncryptionHooks(t)
		sentinel := errors.New("gcm failed")
		newGCM = func(cipher.Block) (cipher.AEAD, error) {
			return nil, sentinel
		}
		if _, err := Encrypt([]byte("data"), "key"); err == nil || !strings.Contains(err.Error(), "error creating GCM") {
			t.Fatalf("Encrypt GCM error = %v", err)
		}
	})

	t.Run("nonce generation", func(t *testing.T) {
		resetEncryptionHooks(t)
		randReader = errReader{err: io.ErrUnexpectedEOF}
		if _, err := Encrypt([]byte("data"), "key"); err == nil || !strings.Contains(err.Error(), "error generating nonce") {
			t.Fatalf("Encrypt nonce error = %v", err)
		}
	})
}

func TestDecryptErrors(t *testing.T) {
	t.Run("empty key", func(t *testing.T) {
		resetEncryptionHooks(t)
		if _, err := Decrypt([]byte("data"), ""); err == nil || err.Error() != "key cannot be empty" {
			t.Fatalf("Decrypt empty key error = %v", err)
		}
	})

	t.Run("cipher creation", func(t *testing.T) {
		resetEncryptionHooks(t)
		sentinel := errors.New("cipher failed")
		newCipher = func([]byte) (cipher.Block, error) {
			return nil, sentinel
		}
		if _, err := Decrypt([]byte("data"), "key"); err == nil || !strings.Contains(err.Error(), "error creating cipher") {
			t.Fatalf("Decrypt cipher error = %v", err)
		}
	})

	t.Run("gcm creation", func(t *testing.T) {
		resetEncryptionHooks(t)
		sentinel := errors.New("gcm failed")
		newGCM = func(cipher.Block) (cipher.AEAD, error) {
			return nil, sentinel
		}
		if _, err := Decrypt([]byte("data"), "key"); err == nil || !strings.Contains(err.Error(), "error creating GCM") {
			t.Fatalf("Decrypt GCM error = %v", err)
		}
	})

	t.Run("too short", func(t *testing.T) {
		resetEncryptionHooks(t)
		if _, err := Decrypt([]byte("short"), "key"); err == nil || err.Error() != "ciphertext too short" {
			t.Fatalf("Decrypt short ciphertext error = %v", err)
		}
	})

	t.Run("authentication failure", func(t *testing.T) {
		resetEncryptionHooks(t)
		ciphertext, err := Encrypt([]byte("data"), "key")
		if err != nil {
			t.Fatal(err)
		}
		ciphertext[len(ciphertext)-1] ^= 0x01

		if _, err := Decrypt(ciphertext, "key"); err == nil || !strings.Contains(err.Error(), "error decrypting") {
			t.Fatalf("Decrypt tampered ciphertext error = %v", err)
		}
	})
}

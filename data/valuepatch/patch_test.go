package valuepatch

import (
	"bytes"
	"errors"
	"testing"
)

func TestApplyOperations(t *testing.T) {
	got, err := Apply([]byte("hello world"), []Operation{
		{Offset: 5, Insert: ","},
		{Offset: 7, Delete: 5, Insert: "TsuDB"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !bytes.Equal(got, []byte("hello, TsuDB")) {
		t.Fatalf("Apply() = %q", got)
	}
}

func TestApplyBase64Insert(t *testing.T) {
	got, err := Apply([]byte{1, 4}, []Operation{
		{Offset: 1, InsertBase64: "AgM="},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Fatalf("Apply() = %v", got)
	}
}

func TestApplyValidation(t *testing.T) {
	if _, err := Apply([]byte("abc"), nil); !errors.Is(err, ErrNoOps) {
		t.Fatalf("empty ops error = %v", err)
	}
	if _, err := Apply([]byte("abc"), []Operation{{Offset: 4}}); !errors.Is(err, ErrInvalidOffset) {
		t.Fatalf("offset error = %v", err)
	}
	if _, err := Apply([]byte("abc"), []Operation{{Offset: 1, Delete: 4}}); !errors.Is(err, ErrInvalidDelete) {
		t.Fatalf("delete error = %v", err)
	}
	if _, err := Apply([]byte("abc"), []Operation{{Offset: 1, Insert: "x", InsertBase64: "eA=="}}); !errors.Is(err, ErrInvalidInsert) {
		t.Fatalf("insert error = %v", err)
	}
}

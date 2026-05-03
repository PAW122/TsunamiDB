package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.RemoveAll("db")
	code := m.Run()
	_ = os.RemoveAll("db")
	os.Exit(code)
}

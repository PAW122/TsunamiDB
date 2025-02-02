package types

type Decoded struct {
	Version      int
	Data         string
	Length       int
	StartPointer int
	EndPointer   int
}

type Encoded struct {
	Version      int
	StartPointer int
	EndPointer   int
}

package types

type Decoded struct {
	Version      int
	Data         string
	Length       int
	StartPointer int
	EndPointer   int
}

type Encoded struct {
	Version      int // version uint8
	StartPointer int // points to begining od data
	EndPointer   int // points to end of data
}

package types

type Decoded struct {
	Version      int
	Data         string
	Length       int
	StartPointer int
	EndPointer   int
	HasNested    bool
}

type Encoded struct {
	Version      int  // version uint8
	StartPointer int  // points to begining od data
	EndPointer   int  // points to end of data
	HasNested    bool // indicates entry contains nested values
}

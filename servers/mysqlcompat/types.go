package mysqlcompat

const (
	defaultDatabase = "tsunamidb"

	comQuit      = 0x01
	comInitDB    = 0x02
	comQuery     = 0x03
	comFieldList = 0x04
	comPing      = 0x0e

	clientLongPassword               uint32 = 1 << 0
	clientFoundRows                  uint32 = 1 << 1
	clientLongFlag                   uint32 = 1 << 2
	clientConnectWithDB              uint32 = 1 << 3
	clientProtocol41                 uint32 = 1 << 9
	clientTransactions               uint32 = 1 << 13
	clientSecureConnection           uint32 = 1 << 15
	clientPluginAuth                 uint32 = 1 << 19
	clientConnectAttrs               uint32 = 1 << 20
	clientPluginAuthLenencClientData uint32 = 1 << 21
	clientSessionTrack               uint32 = 1 << 23
	clientDeprecateEOF               uint32 = 1 << 24
)

const (
	serverStatusAutocommit uint16 = 0x0002
)

const (
	columnTypeDecimal   byte = 0x00
	columnTypeTiny      byte = 0x01
	columnTypeLong      byte = 0x03
	columnTypeFloat     byte = 0x04
	columnTypeDouble    byte = 0x05
	columnTypeNull      byte = 0x06
	columnTypeLongLong  byte = 0x08
	columnTypeVarString byte = 0xfd
)

const (
	columnFlagNotNull  uint16 = 0x0001
	columnFlagPriKey   uint16 = 0x0002
	columnFlagUnsigned uint16 = 0x0020
)

type column struct {
	schema   string
	table    string
	orgTable string
	name     string
	orgName  string
	typ      byte
	flags    uint16
	length   uint32
	decimals byte
}

type queryResult struct {
	columns      []column
	rows         [][]any
	affectedRows uint64
	insertID     uint64
}

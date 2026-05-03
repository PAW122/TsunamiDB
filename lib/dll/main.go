package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"unsafe"

	db "github.com/PAW122/TsunamiDB/lib/dbclient"
)

//export Save
func Save(key, table *C.char, data *C.char, length C.int) C.int {
	if key == nil || table == nil || length < 0 || (length > 0 && data == nil) {
		return -1
	}

	if err := db.Save(C.GoString(key), C.GoString(table), C.GoBytes(unsafe.Pointer(data), length)); err != nil {
		return -1
	}
	return 0
}

//export Read
func Read(key, table *C.char, out **C.char, outLen *C.int) C.int {
	if key == nil || table == nil || out == nil || outLen == nil {
		return -1
	}
	*out = nil
	*outLen = 0

	data, err := db.Read(C.GoString(key), C.GoString(table))
	if err != nil {
		return -1
	}

	buf := (*C.char)(C.CBytes(data))
	if len(data) > 0 && buf == nil {
		return -1
	}
	*outLen = C.int(len(data))
	*out = buf
	return 0
}

//export FreeBuf
func FreeBuf(p *C.char) {
	C.free(unsafe.Pointer(p))
}

//export Free
func Free(key, table *C.char) C.int {
	if key == nil || table == nil {
		return -1
	}

	if err := db.Free(C.GoString(key), C.GoString(table)); err != nil {
		return -1
	}
	return 0
}

//export SaveEncrypted
func SaveEncrypted(key, table, encryptionKey *C.char, data *C.char, length C.int) C.int {
	if key == nil || table == nil || encryptionKey == nil || length < 0 || (length > 0 && data == nil) {
		return -1
	}

	if err := db.SaveEncrypted(
		C.GoString(key), C.GoString(table), C.GoString(encryptionKey),
		C.GoBytes(unsafe.Pointer(data), length),
	); err != nil {
		return -1
	}
	return 0
}

//export ReadEncrypted
func ReadEncrypted(key, table, encryptionKey *C.char, out **C.char, outLen *C.int) C.int {
	if key == nil || table == nil || encryptionKey == nil || out == nil || outLen == nil {
		return -1
	}
	*out = nil
	*outLen = 0

	data, err := db.ReadEncrypted(C.GoString(key), C.GoString(table), C.GoString(encryptionKey))
	if err != nil {
		return -1
	}

	buf := (*C.char)(C.CBytes(data))
	if len(data) > 0 && buf == nil {
		return -1
	}
	*outLen = C.int(len(data))
	*out = buf
	return 0
}

//export InitNetworkManager
func InitNetworkManager(port C.int, peers **C.char, count C.int) {
	if count < 0 || (count > 0 && peers == nil) {
		return
	}

	peerSlice := make([]string, count)
	peerPtr := uintptr(unsafe.Pointer(peers))
	for i := 0; i < int(count); i++ {
		p := *(**C.char)(unsafe.Pointer(peerPtr + uintptr(i)*unsafe.Sizeof(uintptr(0))))
		peerSlice[i] = C.GoString(p)
	}
	db.InitNetworkManager(int(port), peerSlice)
}

//export InitPublicApi
func InitPublicApi(port C.int) {
	db.InitPublicApi(int(port))
}

//export InitSubscriptionServer
func InitSubscriptionServer(port *C.char) C.int {
	if port == nil {
		return -1
	}

	if err := db.InitSubscriptionServer(C.GoString(port)); err != nil {
		return -1
	}
	return 0
}

//export EnableSubscription
func EnableSubscription(keys **C.char, count C.int, authKey **C.char) C.int {
	if count <= 0 || keys == nil || authKey == nil {
		return -1
	}
	*authKey = nil

	goKeys := make([]string, count)
	ptr := uintptr(unsafe.Pointer(keys))
	for i := 0; i < int(count); i++ {
		p := *(**C.char)(unsafe.Pointer(ptr + uintptr(i)*unsafe.Sizeof(uintptr(0))))
		goKeys[i] = C.GoString(p)
	}

	key, err := db.EnableSubscription(goKeys)
	if err != nil {
		return -1
	}

	cKey := C.CString(key)
	if cKey == nil {
		return -1
	}
	*authKey = cKey
	return 0
}

//export DisableSubscription
func DisableSubscription(key *C.char) C.int {
	if key == nil {
		return -1
	}

	if err := db.DisableSubscription(C.GoString(key)); err != nil {
		return -1
	}
	return 0
}

//export GetKeysByRegex
func GetKeysByRegex(table, regex *C.char, max C.int, result ***C.char, count *C.int) C.int {
	if table == nil || regex == nil || result == nil || count == nil || max < 0 {
		return -1
	}
	*result = nil
	*count = 0

	keys, err := db.GetKeysByRegex(C.GoString(table), C.GoString(regex), int(max))
	if err != nil {
		return -1
	}

	*count = C.int(len(keys))
	if len(keys) == 0 {
		*result = nil
		return 0
	}

	ptrArray := C.malloc(C.size_t(len(keys)) * C.size_t(unsafe.Sizeof(uintptr(0))))
	if ptrArray == nil {
		return -1
	}

	for i, key := range keys {
		cstr := C.CString(key)
		if cstr == nil {
			FreeKeysArray((**C.char)(ptrArray), C.int(i))
			*result = nil
			*count = 0
			return -1
		}
		offset := uintptr(i) * unsafe.Sizeof(uintptr(0))
		*(**C.char)(unsafe.Pointer(uintptr(ptrArray) + offset)) = cstr
	}
	*result = (**C.char)(ptrArray)
	return 0
}

//export FreeKeysArray
func FreeKeysArray(array **C.char, count C.int) {
	if array == nil {
		return
	}

	for i := 0; i < int(count); i++ {
		ptr := *(**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(array)) + uintptr(i)*unsafe.Sizeof(uintptr(0))))
		C.free(unsafe.Pointer(ptr))
	}
	C.free(unsafe.Pointer(array))
}

func main() {}

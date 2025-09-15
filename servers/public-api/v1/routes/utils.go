package routes

import (
	"strings"
	"sync"
)

var saveWG sync.WaitGroup
var readWG sync.WaitGroup

func ParseArgs(path, endpoint string) []string {
	trimmed := strings.TrimPrefix(path, "/"+endpoint+"/")
	parts := strings.Split(trimmed, "/")
	prefix := []string{"", endpoint}
	return append(prefix, parts...)
}

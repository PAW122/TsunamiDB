package routes

import (
	"fmt"
	"strings"
	"sync"
)

var saveWG sync.WaitGroup
var readWG sync.WaitGroup

func ParseArgs(path, endpoint string) []string {
	pathParts := strings.Split(strings.TrimPrefix(path, fmt.Sprint("/%s/", endpoint)), "/")
	return pathParts
}

package routes

import (
	"fmt"
	"strings"
)

func ParseArgs(path, endpoint string) []string {
	pathParts := strings.Split(strings.TrimPrefix(path, fmt.Sprint("/%s/", endpoint)), "/")
	return pathParts
}

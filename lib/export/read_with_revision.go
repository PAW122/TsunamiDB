package export

import (
	"fmt"

	"github.com/PAW122/TsunamiDB/data/revision"
	"github.com/PAW122/TsunamiDB/data/valuepatch"
)

type ReadWithRevisionResult struct {
	Data  []byte
	State revision.State
}

func ReadWithRevision(key, table string) (ReadWithRevisionResult, error) {
	if key == "" || table == "" {
		return ReadWithRevisionResult{}, fmt.Errorf("Invalid key or table value")
	}

	unlock := valuepatch.LockKey(table, key)
	defer unlock()

	fsData, err := getElementByKey(table, key)
	if err != nil {
		return ReadWithRevisionResult{}, fmt.Errorf("error retrieving element from map: %w", err)
	}

	data, err := readDataFromFileAsync(table, int64(fsData.StartPtr), int64(fsData.EndPtr))
	if err != nil {
		return ReadWithRevisionResult{}, err
	}
	state, err := getRevisionState(table, key)
	if err != nil {
		return ReadWithRevisionResult{}, err
	}

	decoded := decode(data)
	return ReadWithRevisionResult{
		Data:  []byte(decoded.Data),
		State: state,
	}, nil
}

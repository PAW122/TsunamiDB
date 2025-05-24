package fileSystem_v1

import (
	"encoding/gob"
	"errors"
	"io"
	"os"
	"regexp"
	"sync"
	"time"
)

const (
	baseMapPath        = "./db/maps/data_map_base.gob"
	deltaL1Path        = "./db/maps/data_map_delta_L1.gob"
	deltaL2Path        = "./db/maps/data_map_delta_L2.gob"
	initialTriggerSize = 64 * 1024 // 64KB
	flushBatchSize     = 50
	flushInterval      = 100 * time.Millisecond
)

var (
	mutex        sync.RWMutex
	dataMap      = make(map[string]GetElement_output)
	mapLoaded    = false
	usingDeltaL1 = true
	merging      = false

	deltaBuffer      []GetElement_output
	deltaBufferMutex sync.Mutex
	deltaFlushCond   = sync.NewCond(&sync.Mutex{})

	mergeTriggerSize = int64(initialTriggerSize)
	mergeTimes       []time.Duration
	deltaFillTimes   []time.Duration
	lastDeltaFill    time.Time

	writeCounter     int
	writesPerSecond  int
	writeCounterLock sync.Mutex
)

type GetElement_output struct {
	Key      string
	FileName string
	StartPtr int
	EndPtr   int
}

func init() {
	_ = loadMap()
	lastDeltaFill = time.Now()
	go deltaFlushWorker()
	go writeCounterResetWorker()
}

func writeCounterResetWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		writeCounterLock.Lock()
		writesPerSecond = writeCounter
		writeCounter = 0
		writeCounterLock.Unlock()
	}
}

func loadMap() error {
	mutex.Lock()
	defer mutex.Unlock()

	if mapLoaded {
		return nil
	}

	readGob := func(path string, target map[string]GetElement_output) {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		dec := gob.NewDecoder(f)
		for {
			var entry GetElement_output
			if err := dec.Decode(&entry); err != nil {
				if err == io.EOF {
					break
				}
				return
			}
			target[entry.Key] = entry
		}
	}

	readGob(baseMapPath, dataMap)
	readGob(deltaL1Path, dataMap)
	readGob(deltaL2Path, dataMap)

	mapLoaded = true
	return nil
}

func currentDeltaPath() string {
	if usingDeltaL1 {
		return deltaL1Path
	}
	return deltaL2Path
}

func writeDeltaBatch(entries []GetElement_output) error {
	path := currentDeltaPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := gob.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

func SaveElementByKey(key, fileName string, startPtr, endPtr int) error {
	entry := GetElement_output{
		Key:      key,
		FileName: fileName,
		StartPtr: startPtr,
		EndPtr:   endPtr,
	}

	// Update map in memory
	mutex.Lock()
	dataMap[key] = entry
	mutex.Unlock()

	// Count writes
	writeCounterLock.Lock()
	writeCounter++
	currentRPS := writesPerSecond
	writeCounterLock.Unlock()

	if currentRPS < 100 {
		// LOW LOAD → write directly to disk
		return writeDeltaBatch([]GetElement_output{entry})
	}

	// HIGH LOAD → use buffer
	deltaBufferMutex.Lock()
	deltaBuffer = append(deltaBuffer, entry)
	if len(deltaBuffer) >= flushBatchSize {
		deltaFlushCond.Signal()
	}
	deltaBufferMutex.Unlock()

	return nil
}

func deltaFlushWorker() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	var flushDurations []time.Duration

	for {
		deltaFlushCond.L.Lock()
		for len(deltaBuffer) < flushBatchSize {
			deltaFlushCond.L.Unlock()
			<-ticker.C
			deltaFlushCond.L.Lock()
		}
		bufferCopy := make([]GetElement_output, len(deltaBuffer))
		copy(bufferCopy, deltaBuffer)
		deltaBuffer = deltaBuffer[:0]
		deltaFlushCond.L.Unlock()

		start := time.Now()
		_ = writeDeltaBatch(bufferCopy)
		duration := time.Since(start)
		_ = duration // you can log this or track flush performance later
		flushDurations = append(flushDurations, duration)
		if len(flushDurations) > 3 {
			flushDurations = flushDurations[1:]
		}

		// Check merge condition
		if !merging && usingDeltaL1 {
			if info, err := os.Stat(deltaL1Path); err == nil && info.Size() > mergeTriggerSize {
				merging = true
				usingDeltaL1 = false
				deltaFillTimes = append(deltaFillTimes, time.Since(lastDeltaFill))
				if len(deltaFillTimes) > 3 {
					deltaFillTimes = deltaFillTimes[1:]
				}
				lastDeltaFill = time.Now()
				go mergeToBase()
			}
		}
	}
}

func mergeToBase() {
	start := time.Now()

	mutex.RLock()
	temp := make(map[string]GetElement_output, len(dataMap))
	for k, v := range dataMap {
		temp[k] = v
	}
	mutex.RUnlock()

	f, err := os.Create(baseMapPath)
	if err != nil {
		merging = false
		return
	}
	enc := gob.NewEncoder(f)
	for _, v := range temp {
		_ = enc.Encode(v)
	}
	f.Close()

	_ = os.Remove(deltaL1Path)

	mutex.Lock()
	usingDeltaL1 = true
	merging = false
	mutex.Unlock()

	duration := time.Since(start)
	mergeTimes = append(mergeTimes, duration)
	if len(mergeTimes) > 3 {
		mergeTimes = mergeTimes[1:]
	}
	adjustMergeTriggerSize()
}

func adjustMergeTriggerSize() {
	if len(mergeTimes) < 3 || len(deltaFillTimes) < 3 {
		return
	}

	var mergeAvg, fillAvg time.Duration
	for _, d := range mergeTimes {
		mergeAvg += d
	}
	mergeAvg /= time.Duration(len(mergeTimes))

	for _, d := range deltaFillTimes {
		fillAvg += d
	}
	fillAvg /= time.Duration(len(deltaFillTimes))

	ratio := float64(mergeAvg) / float64(fillAvg)

	if ratio > 0.25 && mergeTriggerSize < 16*1024*1024 {
		mergeTriggerSize = int64(float64(mergeTriggerSize) * 1.5)
	} else if ratio < 0.05 && mergeTriggerSize > 64*1024 {
		mergeTriggerSize = int64(float64(mergeTriggerSize) * 0.75)
	}
}

func RemoveElementByKey(key string) error {
	mutex.Lock()
	defer mutex.Unlock()
	delete(dataMap, key)
	return nil
}

func GetElementByKey(key string) (*GetElement_output, error) {
	mutex.RLock()
	defer mutex.RUnlock()
	entry, ok := dataMap[key]
	if !ok {
		return nil, errors.New("key not found")
	}
	return &entry, nil
}

func GetKeysByRegex(regex string, max int) ([]string, error) {
	mutex.RLock()
	defer mutex.RUnlock()

	compiledRegex, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}

	var matchingKeys []string
	for key := range dataMap {
		if compiledRegex.MatchString(key) {
			matchingKeys = append(matchingKeys, key)
			if max > 0 && len(matchingKeys) >= max {
				break
			}
		}
	}
	return matchingKeys, nil
}

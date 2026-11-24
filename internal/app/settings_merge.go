package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

var settingsFileLocks sync.Map // map[string]*sync.Mutex

func settingsFileLock(path string) *sync.Mutex {
	lock, _ := settingsFileLocks.LoadOrStore(path, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// mergePreferencesFile applies this model's changes since baseline to the
// latest settings object on disk. Unknown JSON and changes made by another
// window are retained.
func mergePreferencesFile(path string, baseline, current json.RawMessage) error {
	if path == "" {
		return nil
	}
	lock := settingsFileLock(path)
	lock.Lock()
	defer lock.Unlock()

	latest := map[string]any{}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := decodeJSONObject(raw, &latest); err != nil {
			return fmt.Errorf("read settings: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	before := map[string]any{}
	if len(bytes.TrimSpace(baseline)) > 0 {
		if err := decodeJSONObject(baseline, &before); err != nil {
			return fmt.Errorf("decode settings baseline: %w", err)
		}
	}
	after := map[string]any{}
	if err := decodeJSONObject(current, &after); err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	mergePreferenceObjects(latest, before, after)
	merged, err := json.MarshalIndent(latest, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, merged)
}

func decodeJSONObject(raw []byte, out *map[string]any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if *out == nil {
		return errors.New("settings object is null")
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("settings file contains more than one JSON value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// mergePreferenceObjects recursively applies only leaves this window changed.
// Maps merge so two windows can add distinct keys concurrently. Arrays and
// scalar values are one leaf and replace the latest value.
func mergePreferenceObjects(dst, baseline, current map[string]any) {
	keys := make(map[string]bool, len(baseline)+len(current))
	for key := range baseline {
		keys[key] = true
	}
	for key := range current {
		keys[key] = true
	}
	for key := range keys {
		before, beforePresent := baseline[key]
		after, afterPresent := current[key]
		if !afterPresent {
			if beforePresent {
				if beforeMap, ok := before.(map[string]any); ok {
					diskMap, ok := dst[key].(map[string]any)
					if !ok {
						diskMap = map[string]any{}
					}
					mergePreferenceObjects(diskMap, beforeMap, map[string]any{})
					if len(diskMap) == 0 {
						delete(dst, key)
					} else {
						dst[key] = diskMap
					}
					continue
				}
				delete(dst, key)
			}
			continue
		}
		if beforePresent && jsonValuesEqual(before, after) {
			continue
		}
		beforeMap, beforeIsMap := before.(map[string]any)
		afterMap, afterIsMap := after.(map[string]any)
		if afterIsMap {
			if !beforePresent || !beforeIsMap {
				beforeMap = map[string]any{}
			}
			diskMap, ok := dst[key].(map[string]any)
			if !ok {
				diskMap = map[string]any{}
			}
			mergePreferenceObjects(diskMap, beforeMap, afterMap)
			dst[key] = diskMap
			continue
		}
		dst[key] = after
	}
}

func jsonValuesEqual(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && bytes.Equal(left, right)
}

package util

import "encoding/json"

func JSONBValue(raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return nil
	}
	if !json.Valid(raw) {
		return nil
	}
	return string(raw)
}

func JSONBBytes(raw []byte) interface{} {
	if len(raw) == 0 {
		return nil
	}
	if !json.Valid(raw) {
		return nil
	}
	return string(raw)
}

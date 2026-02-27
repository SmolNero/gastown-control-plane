package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/SmolNero/gastown-control-plane/internal/model"
)

func ReadSpoolEvents(spoolDir string) ([]model.Event, []string, error) {
	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var events []model.Event
	var processed []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".sent") {
			continue
		}
		if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		path := filepath.Join(spoolDir, name)
		fileEvents, err := parseEventFile(path)
		if err != nil {
			return nil, nil, err
		}
		if len(fileEvents) > 0 {
			events = append(events, fileEvents...)
			processed = append(processed, path)
		}
	}

	return events, processed, nil
}

func MarkProcessed(path string) error {
	return os.Rename(path, path+".sent")
}

func parseEventFile(path string) ([]model.Event, error) {
	if strings.HasSuffix(path, ".jsonl") {
		return parseJSONLines(path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] == '[' {
		var events []model.Event
		if err := json.Unmarshal(raw, &events); err != nil {
			return nil, err
		}
		return events, nil
	}
	var event model.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, err
	}
	return []model.Event{event}, nil
}

func parseJSONLines(path string) ([]model.Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var events []model.Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event model.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

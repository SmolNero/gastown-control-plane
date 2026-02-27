package agent

import (
	"os"
	"path/filepath"

	"github.com/SmolNero/gastown-control-plane/internal/model"
)

func ScanWorkspace(workspace string) ([]model.Rig, []model.Agent, []model.Hook, error) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return nil, nil, nil, err
	}

	var rigs []model.Rig
	var agents []model.Agent
	var hooks []model.Hook

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rigPath := filepath.Join(workspace, entry.Name())
		if !looksLikeRig(rigPath) {
			continue
		}
		rigName := entry.Name()
		rigs = append(rigs, model.Rig{Name: rigName, Path: rigPath})

		crewPath := filepath.Join(rigPath, "crew")
		crewEntries, _ := os.ReadDir(crewPath)
		for _, crewEntry := range crewEntries {
			if !crewEntry.IsDir() {
				continue
			}
			agents = append(agents, model.Agent{
				Name:   crewEntry.Name(),
				Rig:    rigName,
				Role:   "crew",
				Status: "idle",
			})
		}

		hooksPath := filepath.Join(rigPath, "hooks")
		hookEntries, _ := os.ReadDir(hooksPath)
		for _, hookEntry := range hookEntries {
			if !hookEntry.IsDir() {
				continue
			}
			hooks = append(hooks, model.Hook{
				Name:   hookEntry.Name(),
				Rig:    rigName,
				Status: "unknown",
			})
		}
	}

	return rigs, agents, hooks, nil
}

func looksLikeRig(path string) bool {
	if dirExists(filepath.Join(path, "crew")) || dirExists(filepath.Join(path, "hooks")) {
		return true
	}
	return false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

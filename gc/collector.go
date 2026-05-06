package gc

import (
	"fmt"
	"sync"

	"neo-pkg-llm/logger"
	"neo-pkg-llm/machbase"
)

// Collector tracks artifacts created during an agent session
// and cleans up on failure or session end.
type Collector struct {
	mc                *machbase.Client
	createdTQLs       []string
	createdFolders    []string
	createdDashboards []string
	mu                sync.Mutex
}

// NewCollector creates a new artifact collector.
func NewCollector(mc *machbase.Client) *Collector {
	return &Collector{mc: mc}
}

// TrackTQL records a created TQL file path.
func (c *Collector) TrackTQL(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.createdTQLs = append(c.createdTQLs, path)
}

// TrackFolder records a created folder path.
func (c *Collector) TrackFolder(folder string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.createdFolders = append(c.createdFolders, folder)
}

// TrackDashboard records a created dashboard filename.
func (c *Collector) TrackDashboard(filename string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.createdDashboards = append(c.createdDashboards, filename)
}

// CreatedTQLs returns a copy of tracked TQL file paths.
func (c *Collector) CreatedTQLs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]string, len(c.createdTQLs))
	copy(result, c.createdTQLs)
	return result
}

// CleanupOnFailure removes all tracked artifacts.
// Called when the agent encounters an error or timeout.
func (c *Collector) CleanupOnFailure() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error

	// Delete dashboards first (they reference TQL files)
	for _, dsh := range c.createdDashboards {
		if err := c.mc.DeleteFile(dsh); err != nil {
			errs = append(errs, fmt.Errorf("delete dashboard %s: %w", dsh, err))
		} else {
			logger.Infof("[GC] Deleted dashboard: %s", dsh)
		}
	}

	// Delete TQL files
	for _, tql := range c.createdTQLs {
		if err := c.mc.DeleteFile(tql); err != nil {
			errs = append(errs, fmt.Errorf("delete tql %s: %w", tql, err))
		} else {
			logger.Infof("[GC] Deleted TQL: %s", tql)
		}
	}

	// Delete folders (only if empty, best-effort)
	for _, folder := range c.createdFolders {
		if err := c.mc.DeleteFile(folder); err != nil {
			// Folder might not be empty, that's ok
			logger.Infof("[GC] Could not delete folder %s: %v", folder, err)
		} else {
			logger.Infof("[GC] Deleted folder: %s", folder)
		}
	}

	c.createdTQLs = nil
	c.createdFolders = nil
	c.createdDashboards = nil

	if len(errs) > 0 {
		return fmt.Errorf("gc cleanup had %d errors", len(errs))
	}
	return nil
}

// CleanupOrphanedTQLs removes TQL files that were saved but never added to a dashboard.
func (c *Collector) CleanupOrphanedTQLs(usedPaths []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	usedSet := make(map[string]bool, len(usedPaths))
	for _, p := range usedPaths {
		usedSet[p] = true
	}

	var errs []error
	var remaining []string

	for _, tql := range c.createdTQLs {
		if !usedSet[tql] {
			if err := c.mc.DeleteFile(tql); err != nil {
				errs = append(errs, err)
				remaining = append(remaining, tql)
			} else {
				logger.Infof("[GC] Deleted orphaned TQL: %s", tql)
			}
		} else {
			remaining = append(remaining, tql)
		}
	}

	c.createdTQLs = remaining

	if len(errs) > 0 {
		return fmt.Errorf("gc orphan cleanup had %d errors", len(errs))
	}
	return nil
}

// Reset clears all tracked artifacts without deleting them.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.createdTQLs = nil
	c.createdFolders = nil
	c.createdDashboards = nil
}

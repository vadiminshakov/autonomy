package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/ui"
)

type IndexManager struct {
	index       *Index
	indexPath   string
	projectPath string
	mu          sync.RWMutex
	initialized bool
	building    bool
	ticker      *time.Ticker
	ctx         context.Context
	cancel      context.CancelFunc
	autoRebuild bool
}

var (
	globalIndexManager *IndexManager
	indexManagerOnce   sync.Once
)

func GetIndexManager() *IndexManager {
	indexManagerOnce.Do(func() {
		wd, _ := os.Getwd()
		globalIndexManager = NewIndexManager(wd)
	})

	return globalIndexManager
}

func NewIndexManager(projectPath string) *IndexManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &IndexManager{
		index:       NewIndex(projectPath),
		indexPath:   filepath.Join(projectPath, ".autonomy/index.json"),
		projectPath: projectPath,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (im *IndexManager) Initialize() error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.initialized {
		return nil
	}

	if _, err := os.Stat(im.indexPath); err == nil {
		if err := im.index.LoadFromFile(im.indexPath); err == nil {
			if im.isIndexFresh() {
				im.initialized = true
				return nil
			}
		}
	}

	go im.buildIndex()
	im.initialized = true
	return nil
}

func (im *IndexManager) isIndexFresh() bool {
	if im.index.LastUpdated.IsZero() {
		return false
	}

	latestModTime, err := im.getLatestFileModTime()
	if err != nil {
		return false
	}

	return im.index.LastUpdated.After(latestModTime)
}

func (im *IndexManager) getLatestFileModTime() (time.Time, error) {
	var latest time.Time

	err := filepath.Walk(im.projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if filepath.Base(path) == "vendor" || filepath.Base(path) == "node_modules" || filepath.Base(path) == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		lang := im.index.DetectLanguage(path)
		if lang == LanguageUnknown {
			return nil
		}

		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}

		return nil
	})

	return latest, err
}

func (im *IndexManager) buildIndex() {
	im.mu.Lock()
	if im.building {
		im.mu.Unlock()
		return
	}
	im.building = true
	im.mu.Unlock()

	defer func() {
		im.mu.Lock()
		im.building = false
		im.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- im.index.BuildIndex()
	}()

	select {
	case err := <-done:
		if err != nil {
			return
		}

		if err := im.index.SaveToFile(im.indexPath); err != nil {
			ui.ShowIndexError(fmt.Sprintf("Failed to save index: %v", err))
		}
	case <-ctx.Done():
		ui.ShowIndexWarning("Background indexing timed out")
	}
}

func (im *IndexManager) GetIndex() *Index {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.index
}

func (im *IndexManager) RebuildIndex() error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.building {
		return fmt.Errorf("index is already being built")
	}

	if err := im.index.BuildIndex(); err != nil {
		return fmt.Errorf("failed to build index: %w", err)
	}

	if err := im.index.SaveToFile(im.indexPath); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	return nil
}

func (im *IndexManager) IsBuilding() bool {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.building
}

func (im *IndexManager) GetStats() map[string]any {
	im.mu.RLock()
	defer im.mu.RUnlock()

	stats := im.index.GetStats()
	stats["is_building"] = im.building
	stats["initialized"] = im.initialized
	stats["index_path"] = im.indexPath

	return stats
}

func (im *IndexManager) SearchSymbols(query string, filters ...SearchFilter) []*CodeSymbol {
	im.mu.RLock()
	defer im.mu.RUnlock()

	if !im.initialized {
		return nil
	}

	return im.index.SearchSymbols(query, filters...)
}

func (im *IndexManager) GetSymbol(id string) (*CodeSymbol, bool) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	if !im.initialized {
		return nil, false
	}

	return im.index.GetSymbol(id)
}

func (im *IndexManager) GetSymbolsByPackage(pkg string) []*CodeSymbol {
	im.mu.RLock()
	defer im.mu.RUnlock()

	if !im.initialized {
		return nil
	}

	return im.index.GetSymbolsByPackage(pkg)
}

func (im *IndexManager) GetSymbolsByLanguage(lang Language) []*CodeSymbol {
	im.mu.RLock()
	defer im.mu.RUnlock()

	if !im.initialized {
		return nil
	}

	return im.index.GetSymbolsByLanguage(lang)
}

func (im *IndexManager) GetSymbolsByKind(kind SymbolKind) []*CodeSymbol {
	im.mu.RLock()
	defer im.mu.RUnlock()

	if !im.initialized {
		return nil
	}

	return im.index.GetSymbolsByKind(kind)
}

func (im *IndexManager) StartAutoRebuild() {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.autoRebuild {
		return // already running
	}

	im.ticker = time.NewTicker(20 * time.Minute)
	im.autoRebuild = true

	go im.periodicRebuild()
}

func (im *IndexManager) StopAutoRebuild() {
	im.mu.Lock()
	defer im.mu.Unlock()

	if !im.autoRebuild {
		return
	}

	im.autoRebuild = false
	if im.ticker != nil {
		im.ticker.Stop()
		im.ticker = nil
	}
	im.cancel()

	ui.ShowIndexStatus("Index auto-rebuild stopped")
}

// periodicRebuild runs in background and rebuilds index every 20 minutes
func (im *IndexManager) periodicRebuild() {
	for {
		select {
		case <-im.ticker.C:
			im.mu.RLock()
			if !im.autoRebuild {
				im.mu.RUnlock()
				return
			}
			im.mu.RUnlock()

			go im.RebuildIndex()

		case <-im.ctx.Done():
			return
		}
	}
}

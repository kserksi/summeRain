package worker

import (
	"context"
	"log"
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/summerain/image-gallery/internal/config"
	"gorm.io/gorm"
)

type Manager struct {
	DB     *gorm.DB
	Redis  *redis.Client
	Config *config.Config
}

func NewManager(db *gorm.DB, rdb *redis.Client, cfg *config.Config) *Manager {
	return &Manager{
		DB:     db,
		Redis:  rdb,
		Config: cfg,
	}
}

func (m *Manager) Start(ctx context.Context) {
	var wg sync.WaitGroup

	wg.Add(4)

	go func() {
		defer wg.Done()
		m.runHeartbeatMonitor(ctx)
	}()

	go func() {
		defer wg.Done()
		m.runViewFlusher(ctx)
	}()

	go func() {
		defer wg.Done()
		m.runCleanup(ctx)
	}()

	go func() {
		defer wg.Done()
		m.runUserDeletion(ctx)
	}()

	log.Printf("[worker] all workers started")

	wg.Wait()
	log.Printf("[worker] all workers stopped")
}

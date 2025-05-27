package steamtracker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type SteamTracker struct {
	cfg *Config

	ctx        context.Context
	cancel     context.CancelFunc
	wg         *sync.WaitGroup
	httpClient *http.Client

	db        *gorm.DB
	snowflake *snowflake.Node
}

func New(cfg *Config) (*SteamTracker, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	st := SteamTracker{
		cfg: cfg,

		ctx:        ctx,
		cancel:     cancel,
		wg:         &sync.WaitGroup{},
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	log.Debug().Msg("Initializing SteamTracker with configuration")
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	log.Debug().Msg("Configuration validated successfully")

	log.Debug().Msg("Connecting to database")
	db, err := gorm.Open(sqlite.Open(st.cfg.DatabaseDSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	st.db = db
	log.Debug().Msg("Connected to database successfully")

	if err := st.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate database: %w", err)
	}

	log.Debug().Msg("Creating snowflake node")
	snowflakeNode, err := snowflake.NewNode(st.cfg.SnowflakeNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to create snowflake node: %w", err)
	}
	st.snowflake = snowflakeNode
	log.Debug().Msgf("Created snowflake node with ID: %d", st.cfg.SnowflakeNodeID)

	if err := st.ResetDatabase(); err != nil {
		return nil, fmt.Errorf("failed to reset database: %w", err)
	}

	return &st, nil
}

func (st *SteamTracker) Run() error {
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	go st.task()

	for {
		select {
		case <-ticker.C:
			go st.task()
		case <-stopCh:
			log.Info().Msg("shutting down...")
			return st.Stop()
		}
	}
}

func (st *SteamTracker) Stop() error {
	st.cancel()

	st.wg.Wait()

	return nil
}

func (st *SteamTracker) AutoMigrate() error {
	log.Debug().Msg("Running auto-migration database")
	if err := st.db.AutoMigrate(&Player{}); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	return nil
}

func (st *SteamTracker) ResetDatabase() error {
	if !st.cfg.ResetDatabase {
		log.Debug().Msg("Database reset is disabled, skipping...")
		return nil
	}

	log.Debug().Msg("Resetting database...")
	if err := st.db.Migrator().DropTable(&Player{}); err != nil {
		return fmt.Errorf("failed to drop table: %w", err)
	}

	if err := st.AutoMigrate(); err != nil {
		return fmt.Errorf("failed to auto-migrate database: %w", err)
	}

	log.Debug().Msg("Database reset successfully")
	return nil
}

func (st *SteamTracker) GenerateID() int64 {
	return st.snowflake.Generate().Int64()
}

func (st *SteamTracker) AddPlayer(player *Player) error {
	player.ID = st.GenerateID()
	player.CreatedAt = time.Now()

	log.Debug().
		Str("action", "add_player").
		Int64("steam_id", int64(player.SteamID)).
		Str("persona_name", player.PersonaName).
		Str("persona_state", player.PersonaState.String()).
		Send()

	err := st.db.WithContext(st.ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(player).Error; err != nil {
			return fmt.Errorf("failed to create player in transaction: %w", err)
		}

		return nil
	})

	return err
}

func (st *SteamTracker) task() {
	st.wg.Add(1)
	defer st.wg.Done()

	log.Debug().Msg("Starting task...")

	result, err := GetPlayerSummaries(st.httpClient, st.cfg.SteamAPIKey, st.cfg.SteamID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get player summaries")
		return
	}

	if result.Player() == nil {
		log.Warn().Msg("No player data found")
		return
	}

	player := result.Player()

	if err := st.AddPlayer(player); err != nil {
		log.Error().Err(err).Msg("Failed to add player")
	}
}

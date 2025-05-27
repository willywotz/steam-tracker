package steamtracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
	ln         net.Listener
	hs         *http.Server
	mux        *http.ServeMux
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

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	st.mux = http.NewServeMux()
	st.hs = &http.Server{Handler: st.mux}

	ln, err := net.Listen("tcp", ":"+st.cfg.HTTPPort)
	if err != nil {
		return nil, fmt.Errorf("failed to start HTTP listener on port %s: %w", st.cfg.HTTPPort, err)
	}
	st.ln = ln
	log.Debug().Msgf("HTTP listener started on port %s", st.cfg.HTTPPort)

	db, err := gorm.Open(sqlite.Open(st.cfg.DatabaseDSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	st.db = db
	log.Debug().Msg("Connected to database successfully")

	if err := st.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate database: %w", err)
	}
	log.Debug().Msg("Database auto-migration completed successfully")

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

	st.mux.HandleFunc("/search_players", st.GetSearchPlayers)
	go func() { _ = st.hs.Serve(st.ln) }()

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := st.hs.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}

	st.wg.Wait()

	return nil
}

var dbModels = []any{&Player{}}

func (st *SteamTracker) AutoMigrate() error {
	if err := st.db.AutoMigrate(dbModels...); err != nil {
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
	if err := st.db.Migrator().DropTable(dbModels...); err != nil {
		return fmt.Errorf("failed to drop table: %w", err)
	}

	if err := st.AutoMigrate(); err != nil {
		return fmt.Errorf("failed to auto-migrate database: %w", err)
	}

	log.Debug().Msg("Database reset completed successfully")
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

type SearchPlayersQuery struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`

	SteamID        *SteamID   `json:"steam_id"`
	StartCreatedAt *time.Time `json:"start_created_at"`
	EndCreatedAt   *time.Time `json:"end_created_at"`
}

type SearchPlayersQueryResult struct {
	TotalCount int64 `json:"totalCount"`
	Page       int   `json:"page"`
	PerPage    int   `json:"perPage"`

	Players []*Player `json:"players"`
}

func (st *SteamTracker) SearchPlayers(ctx context.Context, query *SearchPlayersQuery) (*SearchPlayersQueryResult, error) {
	event := log.Debug().Str("action", "search_players")
	defer func() { event.Send() }()

	result := SearchPlayersQueryResult{
		Page:    query.Page,
		PerPage: query.Limit,

		Players: make([]*Player, 0),
	}

	err := st.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		whereConditions := make([]string, 0)
		whereParams := make([]any, 0)
		ss := tx.Table("(?) as p", tx.Model(&Player{}))

		setOptional(query.SteamID, func(v SteamID) {
			whereConditions = append(whereConditions, "p.steam_id = ?")
			whereParams = append(whereParams, v)
			event.Str("steam_id", v.String())
		})

		setOptional(query.StartCreatedAt, func(v time.Time) {
			whereConditions = append(whereConditions, "p.created_at >= ?")
			whereParams = append(whereParams, v)
			event.Time("start_created_at", v)
		})

		setOptional(query.EndCreatedAt, func(v time.Time) {
			whereConditions = append(whereConditions, "p.created_at <= ?")
			whereParams = append(whereParams, v)
			event.Time("end_created_at", v)
		})

		if len(whereConditions) > 0 {
			ss = ss.Where(strings.Join(whereConditions, " AND "), whereParams...)
		}

		if query.Page > 0 && query.Limit > 0 {
			ss = ss.Offset((query.Page - 1) * query.Limit).Limit(query.Limit)
			event.Int("page", query.Page).Int("limit", query.Limit)
		}

		if err := ss.Count(&result.TotalCount).Error; err != nil {
			return fmt.Errorf("failed to count players: %w", err)
		}

		if err := ss.Find(&result.Players).Error; err != nil {
			return fmt.Errorf("failed to search players: %w", err)
		}

		return nil
	})
	if err != nil {
		event.Err(err)
	}

	return &result, err
}

func (st *SteamTracker) GetSearchPlayers(w http.ResponseWriter, r *http.Request) {
	query := SearchPlayersQuery{}

	if v := r.URL.Query().Get("page"); v != "" {
		page, _ := strconv.Atoi(v)
		query.Page = page
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ := strconv.Atoi(v)
		query.Limit = limit
	}

	if v := r.URL.Query().Get("steam_id"); v != "" {
		steamIDInt, _ := strconv.ParseInt(v, 10, 64)
		steamID := SteamID(steamIDInt)
		query.SteamID = &steamID
	}

	if v := r.URL.Query().Get("start_created_at"); v != "" {
		startCreatedAt, _ := time.Parse(time.RFC3339, v)
		query.StartCreatedAt = &startCreatedAt
	}

	if v := r.URL.Query().Get("end_created_at"); v != "" {
		endCreatedAt, _ := time.Parse(time.RFC3339, v)
		query.EndCreatedAt = &endCreatedAt
	}

	_ = json.NewDecoder(r.Body).Decode(&query)

	result, err := st.SearchPlayers(r.Context(), &query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to search players: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

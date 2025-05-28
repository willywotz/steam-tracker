package steamtracker

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

	db, err := gorm.Open(sqlite.Open(st.cfg.DatabaseDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
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

	writers := []io.Writer{
		&zerolog.FilteredLevelWriter{
			Writer: zerolog.LevelWriterAdapter{Writer: &st},
			Level:  zerolog.DebugLevel,
		},
		&zerolog.FilteredLevelWriter{
			Writer: zerolog.LevelWriterAdapter{Writer: os.Stdout},
			Level:  st.cfg.LogLevel,
		},
		&zerolog.FilteredLevelWriter{
			Writer: zerolog.LevelWriterAdapter{Writer: os.Stderr},
			Level:  zerolog.ErrorLevel,
		},
	}

	log.Logger = log.Output(zerolog.MultiLevelWriter(writers...))

	return &st, nil
}

func (st *SteamTracker) Write(p []byte) (n int, err error) {
	if _, err := st.CreateAuditLog(&CreateAuditLogCommand{
		Raw: JSON(p),
	}); err != nil {
		return 0, fmt.Errorf("failed to write audit log: %w", err)
	}

	return len(p), nil
}

func (st *SteamTracker) Run() error {
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(st.cfg.TaskInterval) * time.Second)
	defer ticker.Stop()

	go st.task()

	st.mux.HandleFunc("/api/players", st.GetSearchPlayers)
	st.mux.HandleFunc("/api/player_events", st.GetSearchPlayerEvents)
	st.mux.HandleFunc("/api/audit_logs", st.GetSearchAuditLogs)
	st.mux.HandleFunc("/", st.GetIndex)
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

var dbModels = []any{&Player{}, &PlayerEvent{}, &AuditLog{}}

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

func (st *SteamTracker) CreateAuditLog(cmd *CreateAuditLogCommand) (*AuditLog, error) {
	auditLog := cmd.AuditLog()
	auditLog.ID = st.GenerateID()
	auditLog.CreatedAt = time.Now()

	err := st.db.WithContext(st.ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&auditLog).Error; err != nil {
			return fmt.Errorf("failed to create audit log: %w", err)
		}

		return nil
	})

	return &auditLog, err
}

func (st *SteamTracker) AddPlayer(player *Player) error {
	event := log.Debug().
		Str("action", "add_player").
		Int64("steam_id", int64(player.SteamID)).
		Str("persona_name", player.PersonaName).
		Str("persona_state", player.PersonaState.String())
	defer func() { event.Send() }()

	player.ID = st.GenerateID()
	event.Int64("id", player.ID)
	player.CreatedAt = time.Now()
	event.Time("created_at", player.CreatedAt)

	err := st.db.WithContext(st.ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(player).Error; err != nil {
			return fmt.Errorf("failed to create player in transaction: %w", err)
		}

		return nil
	})
	if err != nil {
		event.Err(err)
	}

	return err
}

func (st *SteamTracker) CreatePlayerEvent(cmd *CreatePlayerEventCommand) (*PlayerEvent, error) {
	event := log.Debug().
		Str("action", "create_player_event").
		Int64("steam_id", int64(cmd.SteamID)).
		Str("persona_name", cmd.PersonaName).
		Str("persona_state", cmd.PersonaState.String())
	defer func() { event.Send() }()

	playerEvent := cmd.PlayerEvent()
	playerEvent.ID = st.GenerateID()
	event.Int64("id", playerEvent.ID)
	playerEvent.CreatedAt = time.Now()
	event.Time("created_at", playerEvent.CreatedAt)

	err := st.db.WithContext(st.ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&playerEvent).Error; err != nil {
			return fmt.Errorf("failed to create player event: %w", err)
		}

		return nil
	})
	if err != nil {
		event.Err(err)
	}

	return &playerEvent, err
}

func (st *SteamTracker) GetLatestPlayerEvent(query *GetLatestPlayerEventQuery) (*PlayerEvent, error) {
	event := log.Debug().
		Str("action", "get_latest_player_event").
		Int64("steam_id", int64(query.SteamID))
	defer func() { event.Send() }()

	playerEvent := PlayerEvent{
		PersonaState: PersonaStateUnknown,
	}

	err := st.db.WithContext(st.ctx).Transaction(func(tx *gorm.DB) error {
		ss := tx.Table("(?) as p", tx.Model(&PlayerEvent{}))

		ss = ss.Where("steam_id = ?", query.SteamID)
		ss = ss.Order("created_at DESC")

		if err := ss.First(&playerEvent).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // No events found, return empty PlayerEvent
		} else if err != nil {
			return fmt.Errorf("failed to get latest player event: %w", err)
		}

		return nil
	})
	if err != nil {
		event.Err(err)
	}

	return &playerEvent, err
}

func (st *SteamTracker) task() {
	if st.cfg.DisableTask {
		log.Debug().Msg("Task is disabled, skipping...")
		return
	}

	st.wg.Add(1)
	defer st.wg.Done()

	log.Debug().Msg("Starting task...")

	result, err := GetPlayerSummaries(st.httpClient, st.cfg.SteamAPIKey, st.cfg.SteamID, st.cfg.MaxTaskRetryCount)
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

	latestEvent, err := st.GetLatestPlayerEvent(&GetLatestPlayerEventQuery{
		SteamID: player.SteamID,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to get latest player event")
		return
	}
	if latestEvent.PersonaState == player.PersonaState {
		return
	}
	if _, err := st.CreatePlayerEvent(&CreatePlayerEventCommand{
		SteamID:      player.SteamID,
		PersonaName:  player.PersonaName,
		PersonaState: player.PersonaState,
	}); err != nil {
		log.Error().Err(err).Msg("Failed to create player event")
		return
	}
}

func (st *SteamTracker) SearchPlayers(ctx context.Context, query *SearchPlayersQuery) (*SearchPlayersQueryResult, error) {
	event := log.Debug().Str("action", "search_players")
	defer func() { event.Send() }()

	result := SearchPlayersQueryResult{
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

		if err := ss.Count(&result.TotalCount).Error; err != nil {
			return fmt.Errorf("failed to count players: %w", err)
		}

		setOptional(query.SortBy.CreatedAt, func(order string) {
			ss = ss.Order("p.created_at " + order)
			event.Str("sort_by_created_at", order)
		})

		if query.Page > 0 && query.Limit > 0 {
			result.Page = query.Page
			result.PerPage = query.Limit
			ss = ss.Offset((query.Page - 1) * query.Limit).Limit(query.Limit)
			event.Int("page", query.Page).Int("limit", query.Limit)
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

	if v := r.URL.Query().Get("sort_by[created_at]"); v != "" {
		sortOrder := strings.ToLower(v)
		query.SortBy.CreatedAt = &sortOrder
	}

	_ = json.NewDecoder(r.Body).Decode(&query)

	if err := query.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid query parameters: %v", err), http.StatusBadRequest)
		return
	}

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

func (st *SteamTracker) SearchPlayerEvents(query *SearchPlayerEventsQuery) (*SearchPlayerEventsQueryResult, error) {
	event := log.Debug().Str("action", "search_player_events")
	defer func() { event.Send() }()

	result := SearchPlayerEventsQueryResult{
		PlayerEvents: make([]*PlayerEvent, 0),
	}

	err := st.db.WithContext(st.ctx).Transaction(func(tx *gorm.DB) error {
		whereConditions := make([]string, 0)
		whereParams := make([]any, 0)
		ss := tx.Table("(?) as pe", tx.Model(&PlayerEvent{}))

		setOptional(query.SteamID, func(v SteamID) {
			whereConditions = append(whereConditions, "pe.steam_id = ?")
			whereParams = append(whereParams, v)
			event.Str("steam_id", v.String())
		})

		if len(whereConditions) > 0 {
			ss = ss.Where(strings.Join(whereConditions, " AND "), whereParams...)
		}

		if err := ss.Count(&result.TotalCount).Error; err != nil {
			return fmt.Errorf("failed to count player events: %w", err)
		}

		setOptional(query.SortBy.CreatedAt, func(order string) {
			ss = ss.Order("pe.created_at " + order)
			event.Str("sort_by_created_at", order)
		})

		if query.Page > 0 && query.Limit > 0 {
			result.Page = query.Page
			result.PerPage = query.Limit
			ss = ss.Offset((query.Page - 1) * query.Limit).Limit(query.Limit)
			event.Int("page", query.Page).Int("limit", query.Limit)
		}

		if err := ss.Find(&result.PlayerEvents).Error; err != nil {
			return fmt.Errorf("failed to search player events: %w", err)
		}

		return nil
	})
	if err != nil {
		event.Err(err)
	}

	return &result, err
}

func (st *SteamTracker) GetSearchPlayerEvents(w http.ResponseWriter, r *http.Request) {
	query := SearchPlayerEventsQuery{}

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

	if v := r.URL.Query().Get("sort_by[created_at]"); v != "" {
		sortOrder := strings.ToLower(v)
		query.SortBy.CreatedAt = &sortOrder
	}

	_ = json.NewDecoder(r.Body).Decode(&query)

	if err := query.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid query parameters: %v", err), http.StatusBadRequest)
		return
	}

	result, err := st.SearchPlayerEvents(&query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to search player events: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

func (st *SteamTracker) SearchAuditLogs(query *SearchAuditLogsQuery) (*SearchAuditLogsQueryResult, error) {
	event := log.Debug().Str("action", "search_audit_logs")
	defer func() { event.Send() }()

	result := SearchAuditLogsQueryResult{
		AuditLogs: make([]*AuditLog, 0),
	}

	err := st.db.WithContext(st.ctx).Transaction(func(tx *gorm.DB) error {
		ss := tx.Table("(?) as al", tx.Model(&AuditLog{}))

		if err := ss.Count(&result.TotalCount).Error; err != nil {
			return fmt.Errorf("failed to count audit logs: %w", err)
		}

		if query.Page > 0 && query.Limit > 0 {
			result.Page = query.Page
			result.PerPage = query.Limit
			ss = ss.Offset((query.Page - 1) * query.Limit).Limit(query.Limit)
			event.Int("page", query.Page).Int("limit", query.Limit)
		}

		setOptional(query.SortBy.ID, func(order string) {
			ss = ss.Order("al.id " + order)
			event.Str("sort_by_id", order)
		})

		if err := ss.Find(&result.AuditLogs).Error; err != nil {
			return fmt.Errorf("failed to search audit logs: %w", err)
		}

		return nil
	})
	if err != nil {
		event.Err(err)
	}

	return &result, err
}

func (st *SteamTracker) GetSearchAuditLogs(w http.ResponseWriter, r *http.Request) {
	query := SearchAuditLogsQuery{}

	if v := r.URL.Query().Get("page"); v != "" {
		page, _ := strconv.Atoi(v)
		query.Page = page
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ := strconv.Atoi(v)
		query.Limit = limit
	}

	if v := r.URL.Query().Get("sort_by[id]"); v != "" {
		sortOrder := strings.ToLower(v)
		query.SortBy.ID = &sortOrder
	}

	_ = json.NewDecoder(r.Body).Decode(&query)

	if err := query.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid query parameters: %v", err), http.StatusBadRequest)
		return
	}

	result, err := st.SearchAuditLogs(&query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to search audit logs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

//go:embed templates/*
var fs embed.FS

func (st *SteamTracker) GetIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := fs.ReadFile("templates/index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read template: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(tmpl); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
		return
	}
}

package steamtracker

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type Player struct {
	ID      int64   `json:"id" gorm:"primaryKey"`
	SteamID SteamID `json:"steam_id" gorm:"index"`
	// CommunityVisibilityState int          `json:"community_visibility_state"`
	ProfileState int    `json:"profile_state"`
	PersonaName  string `json:"persona_name"`
	// ProfileUrl   string `json:"profile_url"`
	// Avatar                   string       `json:"avatar"`
	// AvatarMedium             string       `json:"avatar_medium"`
	// AvatarFull               string       `json:"avatar_full"`
	AvatarHash   string       `json:"avatar_hash"`
	LastLogoff   int          `json:"last_logoff"`
	PersonaState PersonaState `json:"persona_state"`
	// PrimaryClanID     string       `json:"primary_clan_id"`
	// TimeCreated int `json:"time_created"`
	// PersonaStateFlags int       `json:"persona_state_flags"`
	// GameExtraInfo     string    `json:"game_extra_info"`
	// GameID            string    `json:"game_id"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

type SteamID int64

func (s SteamID) String() string {
	return fmt.Sprintf("%d", s)
}

func (s SteamID) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *SteamID) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		*s = SteamID(int64(value))
	case string:
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid SteamID format: %s", value)
		}
		*s = SteamID(id)
	default:
		return fmt.Errorf("invalid type for SteamID: %T", v)
	}
	return nil
}

const (
	PersonaStateUnknown PersonaState = iota - 1 // -1 to handle unknown state
	PersonaStateOffline
	PersonaStateOnline
	PersonaStateBusy
	PersonaStateAway
	PersonaStateSnooze
	PersonaStateLookingToTrade
	PersonaStateLookingToPlay
)

var personaStateNames = map[PersonaState]string{
	PersonaStateUnknown:        "Unknown",
	PersonaStateOffline:        "Offline",
	PersonaStateOnline:         "Online",
	PersonaStateBusy:           "Busy",
	PersonaStateAway:           "Away",
	PersonaStateSnooze:         "Snooze",
	PersonaStateLookingToTrade: "Looking to Trade",
	PersonaStateLookingToPlay:  "Looking to Play",
}

type PersonaState int

func (ps PersonaState) String() string {
	if name, exists := personaStateNames[ps]; exists {
		return name
	}
	return "Unknown"
}

func (ps PersonaState) MarshalJSON() ([]byte, error) {
	if name, exists := personaStateNames[ps]; exists {
		return []byte(fmt.Sprintf(`"%s"`, name)), nil
	}
	return []byte(`"Unknown"`), nil
}

func (ps *PersonaState) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		if value < 0 || int(value) >= len(personaStateNames) {
			return fmt.Errorf("invalid persona state value: %d", int(value))
		}
		*ps = PersonaState(int(value))
	case string:
		return ps.fromString(value)
	default:
		return fmt.Errorf("invalid type for PersonaState: %T", v)
	}
	return nil
}

func (ps *PersonaState) fromString(stateName string) error {
	for state, name := range personaStateNames {
		if name == stateName {
			*ps = state
			return nil
		}
	}
	return fmt.Errorf("unknown persona state: %s", stateName)
}

type SearchPlayersQuery struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`

	SteamID        *SteamID   `json:"steam_id"`
	StartCreatedAt *time.Time `json:"start_created_at"`
	EndCreatedAt   *time.Time `json:"end_created_at"`

	SortBy struct {
		CreatedAt *string `json:"created_at"`
	} `json:"sort_by"`
}

func (query *SearchPlayersQuery) Validate() error {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Limit < 1 || query.Limit > 100 {
		query.Limit = 25
	}

	if query.SteamID != nil && *query.SteamID < 0 {
		return fmt.Errorf("invalid SteamID: %d", *query.SteamID)
	}

	if query.StartCreatedAt != nil && query.EndCreatedAt != nil && query.StartCreatedAt.After(*query.EndCreatedAt) {
		return fmt.Errorf("start_created_at cannot be after end_created_at")
	}

	if query.SortBy.CreatedAt != nil {
		if *query.SortBy.CreatedAt != "asc" && *query.SortBy.CreatedAt != "desc" {
			return fmt.Errorf("invalid sort order for created_at: %s, must be 'asc' or 'desc'", *query.SortBy.CreatedAt)
		}
	}

	return nil
}

type SearchPlayersQueryResult struct {
	TotalCount int64 `json:"totalCount"`
	Page       int   `json:"page"`
	PerPage    int   `json:"perPage"`

	Players []*Player `json:"players"`
}

type PlayerEvent struct {
	ID           int64        `json:"id" gorm:"primaryKey"`
	SteamID      SteamID      `json:"steam_id"`
	PersonaName  string       `json:"persona_name"`
	PersonaState PersonaState `json:"persona_state"`
	CreatedAt    time.Time    `json:"created_at"`
}

type CreatePlayerEventCommand struct {
	SteamID      SteamID      `json:"steam_id"`
	PersonaName  string       `json:"persona_name"`
	PersonaState PersonaState `json:"persona_state"`
}

type GetLatestPlayerEventQuery struct {
	SteamID SteamID `json:"steam_id"`
}

type SearchPlayerEventsQuery struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`

	SteamID *SteamID `json:"steam_id"`

	SortBy struct {
		CreatedAt *string `json:"created_at"`
	} `json:"sort_by"`
}

func (query *SearchPlayerEventsQuery) Validate() error {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Limit < 1 || query.Limit > 100 {
		query.Limit = 25
	}

	if query.SteamID != nil && *query.SteamID < 0 {
		return fmt.Errorf("invalid SteamID: %d", *query.SteamID)
	}

	if query.SortBy.CreatedAt != nil {
		if *query.SortBy.CreatedAt != "asc" && *query.SortBy.CreatedAt != "desc" {
			return fmt.Errorf("invalid sort order for created_at: %s, must be 'asc' or 'desc'", *query.SortBy.CreatedAt)
		}
	}

	return nil
}

type SearchPlayerEventsQueryResult struct {
	TotalCount int64 `json:"total_count"`
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`

	PlayerEvents []*PlayerEvent `json:"player_events"`
}

package steamtracker

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type Player struct {
	ID                       int64        `json:"id" gorm:"primaryKey"`
	SteamID                  SteamID      `json:"steam_id" gorm:"index"`
	CommunityVisibilityState int          `json:"community_visibility_state"`
	ProfileState             int          `json:"profile_state"`
	PersonaName              string       `json:"persona_name"`
	ProfileUrl               string       `json:"profile_url"`
	Avatar                   string       `json:"avatar"`
	AvatarMedium             string       `json:"avatar_medium"`
	AvatarFull               string       `json:"avatar_full"`
	AvatarHash               string       `json:"avatar_hash"`
	LastLogoff               int          `json:"last_logoff"`
	PersonaState             PersonaState `json:"persona_state"`
	PrimaryClanID            string       `json:"primary_clan_id"`
	TimeCreated              int          `json:"time_created"`
	PersonaStateFlags        int          `json:"persona_state_flags"`
	GameExtraInfo            string       `json:"game_extra_info"`
	GameID                   string       `json:"game_id"`
	CreatedAt                time.Time    `json:"created_at" gorm:"index"`
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
	PersonaStateOffline PersonaState = iota
	PersonaStateOnline
	PersonaStateBusy
	PersonaStateAway
	PersonaStateSnooze
	PersonaStateLookingToTrade
	PersonaStateLookingToPlay
)

var personaStateNames = map[PersonaState]string{
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

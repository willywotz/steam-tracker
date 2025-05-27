package steamtracker

import (
	"fmt"
)

type Config struct {
	DatabaseDSN     string `json:"database_dsn"`
	SnowflakeNodeID int64  `json:"snowflake_node_id"`
	ResetDatabase   bool   `json:"reset_database"`

	SteamAPIKey string `json:"steam_api_key"`
	SteamID     string `json:"steam_id"`
}

func (c *Config) Validate() error {
	if c.DatabaseDSN == "" {
		return fmt.Errorf("database DSN cannot be empty")
	}
	if c.SnowflakeNodeID < 0 || c.SnowflakeNodeID > 1023 {
		return fmt.Errorf("invalid snowflake node ID: %d, must be between 0 and 1023", c.SnowflakeNodeID)
	}
	if c.SteamAPIKey == "" {
		return fmt.Errorf("Steam API key cannot be empty")
	}
	if c.SteamID == "" {
		return fmt.Errorf("Steam ID cannot be empty")
	}

	return nil
}

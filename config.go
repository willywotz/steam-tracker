package steamtracker

import (
	"fmt"
)

type Config struct {
	DatabaseDSN     string `json:"database_dsn"`
	SnowflakeNodeID int64  `json:"snowflake_node_id"`
	ResetDatabase   bool   `json:"reset_database"`
	HTTPPort        string `json:"http_port"`

	SteamAPIKey string `json:"steam_api_key"`
	SteamID     string `json:"steam_id"`

	MaxTaskRetryCount int `json:"max_task_retry_count"`
	TaskInterval      int `json:"task_interval"` // in seconds

	DisableTask bool `json:"disable_task"`
}

func (c *Config) Validate() error {
	if c.DatabaseDSN == "" {
		return fmt.Errorf("database DSN cannot be empty")
	}
	if c.SnowflakeNodeID < 0 || c.SnowflakeNodeID > 1023 {
		return fmt.Errorf("invalid snowflake node ID: %d, must be between 0 and 1023", c.SnowflakeNodeID)
	}
	if c.HTTPPort == "" {
		return fmt.Errorf("HTTP port cannot be empty")
	}
	if c.SteamAPIKey == "" {
		return fmt.Errorf("Steam API key cannot be empty")
	}
	if c.SteamID == "" {
		return fmt.Errorf("Steam ID cannot be empty")
	}
	if c.MaxTaskRetryCount < 1 {
		return fmt.Errorf("max task retry count must be at least 1")
	}
	if c.TaskInterval < 1 {
		return fmt.Errorf("task interval must be at least 1 second")
	}

	return nil
}

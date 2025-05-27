package steamtracker

import (
	"fmt"
)

type Config struct {
	DatabaseDSN     string `json:"database_dsn"`
	SnowflakeNodeID int64  `json:"snowflake_node_id"`
	ResetDatabase   bool   `json:"reset_database"`
}

func (c *Config) Validate() error {
	if c.DatabaseDSN == "" {
		return fmt.Errorf("database DSN cannot be empty")
	}
	if c.SnowflakeNodeID < 0 || c.SnowflakeNodeID > 1023 {
		return fmt.Errorf("invalid snowflake node ID: %d, must be between 0 and 1023", c.SnowflakeNodeID)
	}

	return nil
}

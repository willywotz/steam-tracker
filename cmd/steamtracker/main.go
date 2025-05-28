package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	_ "github.com/joho/godotenv/autoload"
	steamtracker "github.com/willywotz/steam-tracker"
)

func main() {
	cmd := &cli.Command{
		Name:        "steamtracker",
		Usage:       "A command-line tool for tracking Steam player data",
		Description: "This tool allows you to track and manage Steam player data using the Steam API.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "database-dsn", Sources: cli.EnvVars("DATABASE_DSN")},
			&cli.Int64Flag{Name: "snowflake-node-id", Sources: cli.EnvVars("SNOWFLAKE_NODE_ID")},
			&cli.BoolFlag{Name: "reset-database", Sources: cli.EnvVars("RESET_DATABASE")},
			&cli.StringFlag{Name: "http-port", Value: "8080", Sources: cli.EnvVars("HTTP_PORT")},
			&cli.StringFlag{Name: "log-level", Value: "info", Usage: "Set the logging level (debug, info, warn, error, fatal, panic)", Sources: cli.EnvVars("LOG_LEVEL")},
			&cli.StringFlag{Name: "steam-api-key", Sources: cli.EnvVars("STEAM_API_KEY")},
			&cli.StringFlag{Name: "steam-id", Sources: cli.EnvVars("STEAM_ID")},
			&cli.BoolFlag{Name: "disable-task", Sources: cli.EnvVars("DISABLE_TASK")},
			&cli.IntFlag{Name: "max-task-retry-count", Value: 3, Sources: cli.EnvVars("MAX_TASK_RETRY_COUNT")},
			&cli.IntFlag{Name: "task-interval", Value: 60, Sources: cli.EnvVars("TASK_INTERVAL")},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			level, err := zerolog.ParseLevel(cmd.String("log-level"))
			if err != nil {
				return fmt.Errorf("invalid log level: %w", err)
			}
			log.Logger = log.Level(level)

			cfg := &steamtracker.Config{
				DatabaseDSN:       cmd.String("database-dsn"),
				SnowflakeNodeID:   cmd.Int64("snowflake-node-id"),
				ResetDatabase:     cmd.Bool("reset-database"),
				HTTPPort:          cmd.String("http-port"),
				SteamAPIKey:       cmd.String("steam-api-key"),
				SteamID:           cmd.String("steam-id"),
				DisableTask:       cmd.Bool("disable-task"),
				MaxTaskRetryCount: cmd.Int("max-task-retry-count"),
				TaskInterval:      cmd.Int("task-interval"),
				LogLevel:          level,
			}

			log.Info().Msg("Creating SteamTracker instance")
			st, err := steamtracker.New(cfg)
			if err != nil {
				return fmt.Errorf("failed to create SteamTracker instance: %w", err)
			}

			log.Info().Msg("Running SteamTracker")
			if err := st.Run(); err != nil {
				return fmt.Errorf("failed to run SteamTracker: %w", err)
			}

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Err(err).Msg("Failed to run command")
		os.Exit(1)
	}
}

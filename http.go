package steamtracker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func GetPlayerSummaries(client *http.Client, apiKey string, steamID string) (*GetPlayerSummariesResponse, error) {
	if client == nil {
		return nil, fmt.Errorf("HTTP client cannot be nil")
	}

	url := "https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/"
	url += "?key=" + apiKey + "&steamids=" + steamID

	result, err := retry(func() (*GetPlayerSummariesResponse, error) {
		resp, err := client.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to get player summaries: %w", err)
		}
		defer resp.Body.Close()

		var response GetPlayerSummariesResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		return &response, nil
	}, 3)

	return result, err
}

func retry[T any](fn func() (*T, error), retries int) (*T, error) {
	var err error
	var result *T

	for i := 0; i < retries; i++ {
		result, err = fn()
		if err == nil {
			return result, nil
		}
		time.Sleep(5 * time.Second)
	}

	return result, fmt.Errorf("failed after %d retries: %w", retries, err)
}

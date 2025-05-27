package steamtracker

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func GetPlayerSummaries(client *http.Client, apiKey string, steamID string) (*GetPlayerSummariesResponse, error) {
	if client == nil {
		return nil, fmt.Errorf("HTTP client cannot be nil")
	}

	url := "https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/"
	url += "?key=" + apiKey + "&steamids=" + steamID

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
}

package steamtracker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func GetPlayerSummaries(client *http.Client, apiKey string, steamID string, maxRetryCount int) (*GetPlayerSummariesResponse, error) {
	if client == nil {
		return nil, fmt.Errorf("HTTP client cannot be nil")
	}

	url := "https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v0002/"
	url += "?key=" + apiKey + "&steamids=" + steamID

	result, err := retry(func() (*GetPlayerSummariesResponse, error) {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		req.Header.Set("accept-language", "en-US,en;q=0.9,th;q=0.8")
		req.Header.Set("cache-control", "max-age=0")
		req.Header.Set("priority", "u=0, i")
		req.Header.Set("sec-ch-ua", `"Chromium";v="136", "Google Chrome";v="136", "Not.A/Brand";v="99"`)
		req.Header.Set("sec-ch-ua-mobile", "?0")
		req.Header.Set("sec-ch-ua-platform", `"Windows"`)
		req.Header.Set("sec-fetch-dest", "document")
		req.Header.Set("sec-fetch-mode", "navigate")
		req.Header.Set("sec-fetch-site", "cross-site")
		req.Header.Set("sec-fetch-user", "?1")
		req.Header.Set("upgrade-insecure-requests", "1")
		req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to get player summaries: %w", err)
		}
		defer resp.Body.Close()

		var response GetPlayerSummariesResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		return &response, nil
	}, maxRetryCount)

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
		time.Sleep(15 * time.Second)
	}

	return result, fmt.Errorf("failed after %d retries: %w", retries, err)
}

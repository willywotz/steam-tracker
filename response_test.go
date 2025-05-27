package steamtracker_test

import (
	"encoding/json"
	"testing"

	steamtracker "github.com/willywotz/steam-tracker"
)

func TestUnmarshalGetPlayerSummariesResponse(t *testing.T) {
	responseJSON := `{
		"response": {
			"players": [
				{
					"steamid": "12345678901234567",
					"communityvisibilitystate": 3,
					"profilestate": 1,
					"personaname": "Test Player",
					"profileurl": "https://steamcommunity.com/profiles/12345678901234567",
					"avatar": "https://example.com/avatar.jpg",
					"avatarmedium": "https://example.com/avatarmedium.jpg",
					"avatarfull": "https://example.com/avatarfull.jpg",
					"avatarhash": "abcdef1234567890",
					"lastlogoff": 1622547800,
					"personastate": 1,
					"primaryclanid": "103582791429521412",
					"timecreated": 1609459200,
					"personastateflags": 0,
					"gameextrainfo": "Playing a game",
					"gameid": "1234567890"
				}
			]
		}
	}`

	var response steamtracker.GetPlayerSummariesResponse
	err := json.Unmarshal([]byte(responseJSON), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(response.Response.Players) != 1 {
		t.Fatalf("Expected 1 player, got %d", len(response.Response.Players))
	}

	player := response.Response.Players[0]
	if player.SteamID != 12345678901234567 {
		t.Errorf("Expected SteamID '12345678901234567', got '%d'", player.SteamID)
	}
	if player.PersonaState != steamtracker.PersonaStateOnline {
		t.Errorf("Expected PersonaState 'Online', got '%s'", player.PersonaState)
	}

	t.Logf("Unmarshalled player: %+v", player)
}

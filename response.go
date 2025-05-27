package steamtracker

type GetPlayerSummariesResponse struct {
	Response struct {
		Players []struct {
			SteamID                  SteamID      `json:"steamid"`
			CommunityVisibilityState int          `json:"communityvisibilitystate"`
			ProfileState             int          `json:"profilestate"`
			PersonaName              string       `json:"personaname"`
			ProfileUrl               string       `json:"profileurl"`
			Avatar                   string       `json:"avatar"`
			AvatarMedium             string       `json:"avatarmedium"`
			AvatarFull               string       `json:"avatarfull"`
			AvatarHash               string       `json:"avatarhash"`
			LastLogoff               int          `json:"lastlogoff"`
			PersonaState             PersonaState `json:"personastate"`
			PrimaryClanID            string       `json:"primaryclanid"`
			TimeCreated              int          `json:"timecreated"`
			PersonaStateFlags        int          `json:"personastateflags"`
			GameExtraInfo            string       `json:"gameextrainfo"`
			GameID                   string       `json:"gameid"`
		} `json:"players"`
	} `json:"response"`
}

func (r GetPlayerSummariesResponse) Player() *Player {
	if len(r.Response.Players) == 0 {
		return nil
	}
	p := r.Response.Players[0]
	return &Player{
		SteamID: p.SteamID,
		// CommunityVisibilityState: p.CommunityVisibilityState,
		ProfileState: p.ProfileState,
		PersonaName:  p.PersonaName,
		// ProfileUrl:               p.ProfileUrl,
		// Avatar:                   p.Avatar,
		// AvatarMedium:             p.AvatarMedium,
		// AvatarFull:               p.AvatarFull,
		AvatarHash:   p.AvatarHash,
		LastLogoff:   p.LastLogoff,
		PersonaState: p.PersonaState,
		// PrimaryClanID:            p.PrimaryClanID,
		// TimeCreated:              p.TimeCreated,
		// PersonaStateFlags:        p.PersonaStateFlags,
		// GameExtraInfo:            p.GameExtraInfo,
		GameID: p.GameID,
	}
}

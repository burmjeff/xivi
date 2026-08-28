package controllers

import (
	"strconv"
	"strings"
	"xivi/backend/app/models"
)

// scopeAdminPlaylistChannelLogo replaces a provider-controlled logo URL with
// the authenticated, validated Studio asset endpoint before JSON serialization.
func scopeAdminPlaylistChannelLogo(channel *models.PlaylistChannel) {
	if channel == nil || channel.ID < 1 || channel.Logo == nil || strings.TrimSpace(*channel.Logo) == "" {
		if channel != nil {
			channel.Logo = nil
		}
		return
	}
	value := "/api/v2/studio/source-channels/" + strconv.FormatInt(channel.ID, 10) + "/logo"
	channel.Logo = &value
}

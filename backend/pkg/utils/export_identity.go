package utils

import (
	"fmt"
	"strconv"
	"strings"
	"xivi/backend/app/models"
)

// xmlTVChannelID returns the identifier shared by a lineup's M3U and XMLTV
// exports. Provider TVG IDs remain unchanged so existing guide mappings keep
// working. Channels without a provider mapping receive a stable, lineup-scoped
// identifier instead of all colliding on the legacy "xivi" value.
func xmlTVChannelID(lineup models.Template, channel models.TemplateChannel) string {
	if tvgID := xmlTVSourceChannelID(channel); tvgID != "" {
		return tvgID
	}

	identity := strings.TrimSpace(channel.Uuid)
	if identity == "" {
		identity = strconv.FormatInt(channel.ID, 10)
	}
	return fmt.Sprintf("xivi.%d.%s", lineup.ID, identity)
}

func xmlTVSourceChannelID(channel models.TemplateChannel) string {
	if channel.TvgID == nil {
		return ""
	}
	tvgID := strings.TrimSpace(*channel.TvgID)
	// Older Xivi releases used this shared literal as the missing-ID sentinel.
	// It was never a usable channel identity and must be upgraded at export time.
	if strings.EqualFold(tvgID, "xivi") {
		return ""
	}
	return tvgID
}

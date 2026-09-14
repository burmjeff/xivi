package controllers

import (
	"errors"
	"strings"
	"xivi/backend/app/models"
	"xivi/backend/app/queries"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
)

func V2WatchSearch(c *fiber.Ctx) error {
	p, _ := middleware.Principal(c)
	lineup, err := optionalIntQuery(c, "lineup_id")
	if err != nil {
		return v2Error(c, 400, "invalid_lineup", "Invalid lineup.", false)
	}
	if lineup != nil {
		allowed, err := database.Db.UserCanAccessLineup(c.UserContext(), p.UserID, p.Role, *lineup)
		if err != nil {
			return err
		}
		if !allowed {
			return v2Error(c, 404, "lineup_not_found", "Lineup unavailable.", false)
		}
	}
	term := strings.TrimSpace(c.Query("q"))
	if len(term) < 2 || len(term) > 128 {
		return v2Error(c, 400, "invalid_search", "Enter between 2 and 128 characters.", false)
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return v2Error(c, 400, "invalid_cursor", "Restart the expired search page.", false)
	}
	limit = min(limit, 100)
	rows, total, err := database.Db.SearchWatch(c.UserContext(), p, lineup, term, limit, offset)
	if err != nil {
		return err
	}
	cursor, err := nextCursor(c, offset, len(rows), total)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": rows, "total": total, "next_cursor": cursor})
}

func V2WatchPreferences(c *fiber.Ctx) error {
	principal, _ := middleware.Principal(c)
	if c.Method() == fiber.MethodGet {
		p, err := database.Db.GetViewerPreferences(c.UserContext(), principal.UserID)
		if err != nil {
			return err
		}
		return c.JSON(p)
	}
	p := models.EmptyViewerPreferences()
	if len(c.Body()) > 1024*1024 || decodeStrict(c, &p) != nil || p.Revision < 0 || len(p.Favorites) > 20 || len(p.Order) > 10000 || len(p.Hidden) > 10000 {
		return v2Error(c, 400, "invalid_preferences", "Preferences exceed the supported limits.", false)
	}
	keys := map[string]bool{}
	lists := map[string]bool{}
	checkKeys := func(values []string) bool {
		seen := map[string]bool{}
		if len(values) > 10000 {
			return false
		}
		for _, value := range values {
			if seen[value] {
				return false
			}
			seen[value] = true
			keys[value] = true
		}
		return true
	}
	if !checkKeys(p.Order) || !checkKeys(p.Hidden) {
		return v2Error(c, 400, "invalid_preferences", "Channel lists must not contain duplicates.", false)
	}
	for i := range p.Favorites {
		list := &p.Favorites[i]
		list.Name = strings.TrimSpace(list.Name)
		if list.ID == "" || len(list.ID) > 80 || lists[list.ID] || len(list.Name) < 1 || len(list.Name) > 80 || !checkKeys(list.Channels) {
			return v2Error(c, 400, "invalid_preferences", "Each favorites list needs a unique ID, name and channels.", false)
		}
		lists[list.ID] = true
	}
	if len(keys) > 10000 {
		return v2Error(c, 400, "invalid_preferences", "Preferences support at most 10,000 distinct channels.", false)
	}
	pairs := make([][2]int64, 0, len(keys))
	previous, err := database.Db.GetViewerPreferences(c.UserContext(), principal.UserID)
	if err != nil {
		return err
	}
	existingKeys := map[string]bool{}
	for _, key := range previous.Order {
		existingKeys[key] = true
	}
	for _, key := range previous.Hidden {
		existingKeys[key] = true
	}
	for _, list := range previous.Favorites {
		for _, key := range list.Channels {
			existingKeys[key] = true
		}
	}
	for key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) != 2 {
			return v2Error(c, 400, "invalid_preferences", "Invalid channel key.", false)
		}
		l, ok := security.ParsePositiveID(parts[0])
		ch, ok2 := security.ParsePositiveID(parts[1])
		if !ok || !ok2 {
			return v2Error(c, 400, "invalid_preferences", "Invalid channel key.", false)
		}
		// Retain previously saved identities even after a lineup loses access, so
		// unrelated edits/removals are not blocked. New identities require access;
		// saved preferences never grant permission to view a channel.
		if !existingKeys[key] {
			pairs = append(pairs, [2]int64{l, ch})
		}
	}
	valid, err := database.Db.ValidateViewerChannelKeys(c.UserContext(), principal, pairs)
	if err != nil {
		return err
	}
	if !valid {
		return v2Error(c, 403, "channel_unavailable", "Remove channels that are no longer accessible before saving.", false)
	}
	if p.Favorites == nil {
		p.Favorites = []models.FavoriteList{}
	}
	if p.Order == nil {
		p.Order = []string{}
	}
	if p.Hidden == nil {
		p.Hidden = []string{}
	}
	saved, err := database.Db.SaveViewerPreferences(c.UserContext(), principal.UserID, p)
	if errors.Is(err, queries.ErrPreferenceConflict) {
		current, e := database.Db.GetViewerPreferences(c.UserContext(), principal.UserID)
		if e != nil {
			return e
		}
		return c.Status(409).JSON(fiber.Map{"code": "preference_conflict", "message": "Another device updated these preferences. Reloaded its changes; try your edit again.", "current": current})
	}
	if err != nil {
		return err
	}
	return c.JSON(saved)
}

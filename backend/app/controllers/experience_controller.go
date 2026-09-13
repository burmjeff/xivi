package controllers

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/app/queries"
	"xivi/backend/pkg/logoassets"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/security"
	"xivi/backend/pkg/streaming"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/cron"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

func v2Error(c *fiber.Ctx, status int, code, message string, retryable bool) error {
	return c.Status(status).JSON(models.APIError{Code: code, Message: message, Retryable: retryable})
}

func parseID(c *fiber.Ctx, name string) (int64, error) {
	return strconv.ParseInt(c.Params(name), 10, 64)
}

func optionalIntQuery(c *fiber.Ctx, name string) (*int64, error) {
	value := c.Query(name)
	if value == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func optionalBoolQuery(c *fiber.Ctx, name string) (bool, error) {
	value := c.Query(name)
	if value == "" {
		return false, nil
	}
	return strconv.ParseBool(value)
}

func paginationBinding(c *fiber.Ctx) string {
	principal, _ := middleware.Principal(c)
	values := []string{}
	c.Context().QueryArgs().VisitAll(func(key, value []byte) {
		if strings.EqualFold(string(key), "cursor") {
			return
		}
		values = append(values, string(key)+"="+string(value))
	})
	sort.Strings(values)
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return fmt.Sprintf("user:%d|%s|%x", principal.UserID, c.Path(), digest)
}

func pageParams(c *fiber.Ctx) (int, int, error) {
	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	offset := 0
	if cursor := c.Query("cursor"); cursor != "" {
		var err error
		offset, err = security.ParsePageCursor(cursor, paginationBinding(c))
		if err != nil {
			return 0, 0, err
		}
	}
	return limit, offset, nil
}

func nextCursor(c *fiber.Ctx, offset, count int, total int64) (*string, error) {
	next := offset + count
	if int64(next) >= total {
		return nil, nil
	}
	value, err := security.EncodePageCursor(next, paginationBinding(c))
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func scopeGuideChannelAssets(channel *models.GuideChannel, lineupID int64) {
	if channel == nil || channel.Logo == "" {
		return
	}
	separator := "?"
	if strings.Contains(channel.Logo, "?") {
		separator = "&"
	}
	channel.Logo += separator + "lineup_id=" + strconv.FormatInt(lineupID, 10)
}

func scopedSourceLogoURL(sourceChannelID int64) *string {
	if sourceChannelID < 1 {
		return nil
	}
	value := "/api/v2/studio/source-channels/" + strconv.FormatInt(sourceChannelID, 10) + "/logo"
	return &value
}

func timeWindow(c *fiber.Ctx) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	from, to := now.Add(-30*time.Minute), now.Add(6*time.Hour)
	var err error
	if value := c.Query("from"); value != "" {
		from, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return from, to, err
		}
	}
	if value := c.Query("to"); value != "" {
		to, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return from, to, err
		}
	}
	if !to.After(from) {
		return from, to, fmt.Errorf("to must be after from")
	}
	if to.Sub(from) > 72*time.Hour {
		return from, to, fmt.Errorf("guide windows cannot exceed 72 hours")
	}
	return from.UTC(), to.UTC(), nil
}

// V2WatchLineups returns lineups without exposing legacy template terminology.
func V2WatchLineups(c *fiber.Ctx) error {
	principal, _ := middleware.Principal(c)
	var items []models.LineupSummary
	var err error
	if principal.IsAdmin() {
		items, err = database.Db.GetLineupSummaries(c.UserContext())
	} else {
		items, err = database.Db.GetViewerLineupSummaries(c.UserContext(), principal.UserID)
	}
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "lineups_unavailable", "Lineups could not be loaded.", true)
	}
	return c.JSON(fiber.Map{"items": items, "next_cursor": nil, "total": len(items)})
}

func V2WatchLineupGroups(c *fiber.Ctx) error {
	lineupID, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	items, err := database.Db.GetWatchLineupGroups(c.UserContext(), lineupID)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "groups_unavailable", "Groups could not be loaded.", true)
	}
	return c.JSON(fiber.Map{"items": items, "next_cursor": nil, "total": len(items)})
}

func v2LineupChannels(c *fiber.Ctx, guide bool) error {
	lineupID, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	groupID, err := optionalIntQuery(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	from, to, err := timeWindow(c)
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_time_window", err.Error(), false)
	}
	if !guide && c.Query("from") == "" {
		to = time.Now().UTC().Add(4 * time.Hour)
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The page cursor is invalid or expired.", false)
	}
	items, total, err := database.Db.GetGuideChannels(c.UserContext(), lineupID, groupID, strings.TrimSpace(c.Query("q")), from, to, limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "channels_unavailable", "Channels could not be loaded.", true)
	}
	for index := range items {
		scopeGuideChannelAssets(&items[index], lineupID)
	}
	cursor, err := nextCursor(c, offset, len(items), total)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "cursor_unavailable", "The next page could not be created.", true)
	}
	return c.JSON(models.Paginated[models.GuideChannel]{Items: items, NextCursor: cursor, Total: total})
}

func V2WatchChannels(c *fiber.Ctx) error { return v2LineupChannels(c, false) }
func V2WatchGuide(c *fiber.Ctx) error    { return v2LineupChannels(c, true) }

func V2WatchChannel(c *fiber.Ctx) error {
	id, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	now := time.Now().UTC()
	lineupID, queryErr := optionalIntQuery(c, "lineup_id")
	if queryErr != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	if lineupID == nil && !principal.IsAdmin() {
		return v2Error(c, fiber.StatusBadRequest, "lineup_required", "A lineup id is required.", false)
	}
	if lineupID != nil {
		allowed, accessErr := database.Db.UserCanAccessLineup(c.UserContext(), principal.UserID, principal.Role, *lineupID)
		if accessErr != nil || !allowed {
			return v2Error(c, fiber.StatusNotFound, "channel_not_found", "That channel is no longer available.", false)
		}
	}
	var item *models.GuideChannel
	if lineupID != nil {
		item, err = database.Db.GetGuideChannelForLineup(c.UserContext(), *lineupID, id, now.Add(-2*time.Hour), now.Add(12*time.Hour))
	} else {
		item, err = database.Db.GetGuideChannel(c.UserContext(), id, now.Add(-2*time.Hour), now.Add(12*time.Hour))
	}
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "channel_not_found", "That channel is no longer available.", false)
	}
	if lineupID != nil {
		scopeGuideChannelAssets(item, *lineupID)
	}
	return c.JSON(item)
}

func V2WatchChannelNeighbors(c *fiber.Ctx) error {
	lineupID, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	neighbors, err := database.Db.GetWatchChannelNeighbors(c.UserContext(), lineupID, channelID)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "channel_not_found", "That channel is no longer available in this lineup.", false)
	}
	scopeGuideChannelAssets(neighbors.Previous, lineupID)
	scopeGuideChannelAssets(neighbors.Next, lineupID)
	return c.JSON(neighbors)
}

func V2Search(c *fiber.Ctx) error {
	lineupID, err := optionalIntQuery(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	principal, _ := middleware.Principal(c)
	if lineupID == nil {
		var lineups []models.LineupSummary
		var loadErr error
		if principal.IsAdmin() {
			lineups, loadErr = database.Db.GetLineupSummaries(c.UserContext())
		} else {
			lineups, loadErr = database.Db.GetViewerLineupSummaries(c.UserContext(), principal.UserID)
		}
		if loadErr != nil || len(lineups) == 0 {
			return c.JSON(fiber.Map{"items": []any{}, "next_cursor": nil, "total": 0})
		}
		lineupID = &lineups[0].ID
	} else {
		allowed, accessErr := database.Db.UserCanAccessLineup(c.UserContext(), principal.UserID, principal.Role, *lineupID)
		if accessErr != nil || !allowed {
			return v2Error(c, fiber.StatusNotFound, "lineup_not_found", "The lineup was not found.", false)
		}
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The page cursor is invalid or expired.", false)
	}
	now := time.Now().UTC()
	items, total, err := database.Db.GetGuideChannels(c.UserContext(), *lineupID, nil, strings.TrimSpace(c.Query("q")), now.Add(-time.Hour), now.Add(4*time.Hour), limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "search_unavailable", "Search is temporarily unavailable.", true)
	}
	for index := range items {
		scopeGuideChannelAssets(&items[index], *lineupID)
	}
	cursor, err := nextCursor(c, offset, len(items), total)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "cursor_unavailable", "The next page could not be created.", true)
	}
	return c.JSON(models.Paginated[models.GuideChannel]{Items: items, NextCursor: cursor, Total: total})
}

func V2StudioOverview(c *fiber.Ctx) error {
	overview, err := database.Db.GetStudioOverview(c.UserContext())
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "overview_unavailable", "Studio health could not be loaded.", true)
	}
	jobs, _ := database.Db.GetJobs(c.UserContext(), 6)
	return c.JSON(fiber.Map{"summary": overview, "jobs": jobs})
}

func V2StudioCoverage(c *fiber.Ctx) error {
	coverage, err := database.Db.GetCoverageSummary(c.UserContext())
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "coverage_unavailable", "Guide coverage could not be loaded.", true)
	}
	return c.JSON(coverage)
}

func V2StudioLineups(c *fiber.Ctx) error { return V2WatchLineups(c) }

func V2StudioLineupGroups(c *fiber.Ctx) error {
	id, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	items, err := database.Db.GetLineupGroups(c.UserContext(), id)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "groups_unavailable", "Groups could not be loaded.", true)
	}
	return c.JSON(fiber.Map{"items": items, "next_cursor": nil, "total": len(items)})
}

type positionRequest struct {
	BeforeID *int64 `json:"before_id"`
	AfterID  *int64 `json:"after_id"`
}

func V2MoveStudioGroup(c *fiber.Ctx) error {
	lineupID, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	body := positionRequest{}
	if err := decodeStrict(c, &body); err != nil || (body.BeforeID == nil) == (body.AfterID == nil) {
		return v2Error(c, fiber.StatusBadRequest, "invalid_move", "Choose exactly one group to move before or after.", false)
	}
	if err := database.Db.MoveWorkspaceGroup(c.UserContext(), lineupID, groupID, body.BeforeID, body.AfterID); err != nil {
		if errors.Is(err, queries.ErrLineupNotFound) || errors.Is(err, queries.ErrStudioGroupNotFound) {
			return studioGroupError(c, err)
		}
		log.Error().Err(err).Msg("Lineup group move failed")
		return v2Error(c, fiber.StatusUnprocessableEntity, "group_move_failed", "The lineup group could not be moved.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func studioGroupError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, queries.ErrLineupNotFound):
		return v2Error(c, fiber.StatusNotFound, "lineup_not_found", "That lineup no longer exists.", false)
	case errors.Is(err, queries.ErrStudioGroupNotFound):
		return v2Error(c, fiber.StatusNotFound, "group_not_found", "That lineup group no longer exists.", false)
	case errors.Is(err, queries.ErrSourceGroupNotFound):
		return v2Error(c, fiber.StatusNotFound, "source_group_not_found", "That source group is no longer available. Refresh the source list and choose another group.", false)
	case errors.Is(err, queries.ErrSourceGroupDisabled):
		return v2Error(c, fiber.StatusConflict, "source_group_disabled", "That source group is disabled. Enable it before importing channels.", false)
	case errors.Is(err, queries.ErrSourceGroupEmpty):
		return v2Error(c, fiber.StatusConflict, "source_group_empty", "That source group does not contain any channels.", false)
	case errors.Is(err, queries.ErrStudioGroupManaged):
		return v2Error(c, fiber.StatusConflict, "synced_group_managed", "Channels are controlled by this group's source connection.", false)
	case errors.Is(err, queries.ErrStudioGroupName):
		return v2Error(c, fiber.StatusBadRequest, "group_name_required", "Enter a name for the lineup group.", false)
	case strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: templategroup.name"):
		return v2Error(c, fiber.StatusConflict, "group_name_exists", "A lineup group already uses that name.", false)
	case strings.Contains(strings.ToLower(err.Error()), "database is locked"):
		return v2Error(c, fiber.StatusServiceUnavailable, "database_busy", "Xivi is finishing another update. Try again in a moment; no changes were saved.", true)
	default:
		log.Error().Err(err).Msg("Studio group operation failed")
		return v2Error(c, fiber.StatusInternalServerError, "group_operation_failed", "The lineup group could not be saved.", true)
	}
}

func V2CreateStudioGroup(c *fiber.Ctx) error {
	lineupID, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	request := models.StudioGroupCreateRequest{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The lineup group request is invalid.", false)
	}
	request.Name = strings.TrimSpace(request.Name)
	groupID, err := database.Db.CreateStudioGroup(c.UserContext(), lineupID, request)
	if err != nil {
		return studioGroupError(c, err)
	}
	if request.SourceLink == nil {
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"group_id": groupID})
	}
	job, err := launchV2Job(c.UserContext(), "sync", "group", groupID, "Group sync queued.", "Reconciling the source group…", "Synced group updated.", func(report jobProgressFunc) error {
		report(15, "Loading the source group connection…")
		result, err := database.Db.SyncSourceGroup(context.Background(), groupID)
		if err == nil {
			report(92, fmt.Sprintf("Source group synchronized: %d added, %d updated, %d removed.", result.AddedCount, result.UpdatedCount, result.RemovedCount))
		}
		return err
	})
	if err != nil {
		if cleanupErr := database.Db.DeleteStudioGroup(c.UserContext(), groupID); cleanupErr != nil {
			log.Error().Err(cleanupErr).Int64("group_id", groupID).Msg("Failed to roll back a group after job creation failed")
		}
		return v2Error(c, fiber.StatusInternalServerError, "job_unavailable", "The source connection could not be queued; no group was created.", true)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"group_id": groupID, "job_id": job.ID, "status": job.Status})
}

func V2UpdateStudioGroup(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	request := models.StudioGroupUpdateRequest{}
	if err := decodeStrict(c, &request); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The lineup group request is invalid.", false)
	}
	request.Name = strings.TrimSpace(request.Name)
	if err := database.Db.UpdateStudioGroup(c.UserContext(), groupID, request); err != nil {
		return studioGroupError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2DeleteStudioGroup(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	if err := database.Db.DeleteStudioGroup(c.UserContext(), groupID); err != nil {
		return studioGroupError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2CopySourceGroupToLineup(c *fiber.Ctx) error {
	lineupID, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	sourceGroupID, err := parseID(c, "source_group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source_group", "The source group id is invalid.", false)
	}
	result, err := database.Db.CopySourceGroupToLineup(c.UserContext(), lineupID, sourceGroupID)
	if err != nil {
		return studioGroupError(c, err)
	}
	matchImportedChannels(result.ChannelIDs)
	return c.Status(fiber.StatusCreated).JSON(result)
}

func V2AddSourceGroupToStudioGroup(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	sourceGroupID, err := parseID(c, "source_group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source_group", "The source group id is invalid.", false)
	}
	result, err := database.Db.AddSourceGroupToStudioGroup(c.UserContext(), groupID, sourceGroupID)
	if err != nil {
		return studioGroupError(c, err)
	}
	matchImportedChannels(result.ChannelIDs)
	return c.Status(fiber.StatusCreated).JSON(result)
}

func matchImportedChannels(channelIDs []int64) {
	for _, channelID := range channelIDs {
		if channel, err := database.Db.GetTmplChannel(channelID); err == nil {
			go utils.MatchTemplateChannel(channel)
		}
	}
}

func V2StudioSourceGroups(c *fiber.Ctx) error {
	playlistID, err := optionalIntQuery(c, "playlist_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source", "The source id is invalid.", false)
	}
	lineupID, err := optionalIntQuery(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	unusedOnly, err := optionalBoolQuery(c, "unused_only")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_filter", "The source usage filter is invalid.", false)
	}
	if unusedOnly && lineupID == nil {
		return v2Error(c, fiber.StatusBadRequest, "lineup_required", "Choose a lineup before filtering used sources.", false)
	}
	enabledOnly, err := optionalBoolQuery(c, "enabled_only")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_filter", "The source availability filter is invalid.", false)
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The page cursor is invalid or expired.", false)
	}
	items, total, err := database.Db.GetSourceGroups(c.UserContext(), playlistID, lineupID, unusedOnly, enabledOnly, strings.TrimSpace(c.Query("q")), limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "source_groups_unavailable", "Source groups could not be loaded.", true)
	}
	cursor, err := nextCursor(c, offset, len(items), total)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "cursor_unavailable", "The next page could not be created.", true)
	}
	return c.JSON(models.Paginated[models.SourceGroup]{Items: items, NextCursor: cursor, Total: total})
}

func V2SetStudioSourceGroupEnabled(c *fiber.Ctx) error {
	sourceGroupID, err := parseID(c, "source_group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source_group", "The source group id is invalid.", false)
	}
	request := models.SourceGroupEnableRequest{}
	if err := decodeStrict(c, &request); err != nil || request.Enabled == nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_enablement", "The source group enablement request is invalid.", false)
	}
	if err := database.Db.SetSourceGroupEnabled(c.UserContext(), sourceGroupID, *request.Enabled); err != nil {
		if errors.Is(err, queries.ErrSourceGroupNotFound) {
			return studioGroupError(c, err)
		}
		return v2Error(c, fiber.StatusUnprocessableEntity, "enablement_failed", "Source group enablement could not be updated.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2SetStudioGroupSourceLink(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	request := models.SourceGroupLinkRequest{}
	if err := decodeStrict(c, &request); err != nil || request.SourceGroupID < 1 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source_group", "Choose a source group to sync.", false)
	}
	if err := database.Db.SetSourceGroupLink(c.UserContext(), groupID, request); err != nil {
		return studioGroupError(c, err)
	}
	return queueV2Job(c, "sync", "group", groupID, "Group sync queued.", "Reconciling the source group…", "Synced group updated.", func(report jobProgressFunc) error {
		report(15, "Loading the source group connection…")
		result, err := database.Db.SyncSourceGroup(context.Background(), groupID)
		if err == nil {
			report(92, fmt.Sprintf("Source group synchronized: %d added, %d updated, %d removed.", result.AddedCount, result.UpdatedCount, result.RemovedCount))
		}
		return err
	})
}

func V2DeleteStudioGroupSourceLink(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	retainChannels := !strings.EqualFold(c.Query("retain_channels", "true"), "false")
	if err := database.Db.DisconnectSourceGroup(c.UserContext(), groupID, retainChannels); err != nil {
		return v2Error(c, fiber.StatusUnprocessableEntity, "source_disconnect_failed", "The source group could not be disconnected.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2SyncStudioGroup(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	if _, err := database.Db.GetSourceGroupLink(c.UserContext(), groupID); err != nil {
		return v2Error(c, fiber.StatusNotFound, "source_link_not_found", "This group is not connected to a source group.", false)
	}
	return queueV2Job(c, "sync", "group", groupID, "Group sync queued.", "Reconciling the source group…", "Synced group updated.", func(report jobProgressFunc) error {
		report(15, "Loading the source group connection…")
		result, err := database.Db.SyncSourceGroup(context.Background(), groupID)
		if err == nil {
			report(92, fmt.Sprintf("Source group synchronized: %d added, %d updated, %d removed.", result.AddedCount, result.UpdatedCount, result.RemovedCount))
		}
		return err
	})
}

func V2StudioGroupChannels(c *fiber.Ctx) error {
	id, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	matchHealth := strings.TrimSpace(c.Query("match"))
	if matchHealth != "" && matchHealth != "unmatched" && matchHealth != "low-confidence" && matchHealth != "duplicate-tvg-id" {
		return v2Error(c, fiber.StatusBadRequest, "invalid_match_filter", "Choose unmatched, low-confidence, or duplicate guide-ID review.", false)
	}
	lineupID, err := optionalIntQuery(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	if matchHealth == "duplicate-tvg-id" && lineupID == nil {
		return v2Error(c, fiber.StatusBadRequest, "lineup_required", "Choose a lineup for duplicate guide-ID review.", false)
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The page cursor is invalid or expired.", false)
	}
	items, total, err := database.Db.GetWorkspaceChannels(c.UserContext(), id, lineupID, strings.TrimSpace(c.Query("q")), matchHealth, limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "workspace_unavailable", "Lineup channels could not be loaded.", true)
	}
	cursor, err := nextCursor(c, offset, len(items), total)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "cursor_unavailable", "The next page could not be created.", true)
	}
	return c.JSON(models.Paginated[models.WorkspaceChannel]{Items: items, NextCursor: cursor, Total: total})
}

func V2StudioDuplicateTVGIDs(c *fiber.Ctx) error {
	lineupID, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	includeAcknowledged, err := optionalBoolQuery(c, "include_acknowledged")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_filter", "The acknowledgement filter is invalid.", false)
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The page cursor is invalid or expired.", false)
	}
	items, total, err := database.Db.GetDuplicateTVGIDReviews(c.UserContext(), lineupID, includeAcknowledged, limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "duplicate_review_unavailable", "Duplicate guide IDs could not be loaded.", true)
	}
	cursor, err := nextCursor(c, offset, len(items), total)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "cursor_unavailable", "The next page could not be created.", true)
	}
	return c.JSON(models.Paginated[models.DuplicateTVGIDReview]{Items: items, NextCursor: cursor, Total: total})
}

func V2SetStudioDuplicateTVGIDReview(c *fiber.Ctx) error {
	lineupID, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	request := models.DuplicateTVGIDReviewRequest{}
	if err := decodeStrict(c, &request); err != nil || strings.TrimSpace(request.TVGID) == "" {
		return v2Error(c, fiber.StatusBadRequest, "invalid_duplicate_review", "Choose a duplicate guide ID to review.", false)
	}
	err = database.Db.SetDuplicateTVGIDReviewAcknowledged(c.UserContext(), lineupID, request.TVGID, request.Acknowledged)
	if errors.Is(err, queries.ErrDuplicateTVGIDNotFound) {
		return v2Error(c, fiber.StatusNotFound, "duplicate_review_not_found", "That duplicate guide-ID issue is no longer active.", false)
	}
	if err != nil {
		return v2Error(c, fiber.StatusUnprocessableEntity, "duplicate_review_failed", "The duplicate guide-ID review could not be saved.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2MergeStudioDuplicateTVGID(c *fiber.Ctx) error {
	lineupID, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	request := models.DuplicateTVGIDMergeRequest{}
	if err := decodeStrict(c, &request); err != nil || strings.TrimSpace(request.TVGID) == "" || request.KeepChannelID < 1 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_duplicate_merge", "Choose which duplicate channel to keep.", false)
	}
	result, err := database.Db.MergeDuplicateTVGIDChannels(c.UserContext(), lineupID, request.TVGID, request.KeepChannelID)
	switch {
	case errors.Is(err, queries.ErrDuplicateTVGIDNotFound):
		return v2Error(c, fiber.StatusNotFound, "duplicate_review_not_found", "That duplicate guide-ID issue is no longer active.", false)
	case errors.Is(err, queries.ErrDuplicateTVGIDManaged):
		return v2Error(c, fiber.StatusConflict, "duplicate_source_managed", "Disconnect the source-managed group before merging these channels, or allow the shared guide.", false)
	case errors.Is(err, queries.ErrDuplicateMergeTarget):
		return v2Error(c, fiber.StatusBadRequest, "invalid_merge_target", "The channel to keep is not part of this duplicate set.", false)
	case err != nil:
		return v2Error(c, fiber.StatusUnprocessableEntity, "duplicate_merge_failed", "The duplicate channels could not be merged.", false)
	default:
		return c.JSON(result)
	}
}

func V2StudioSourceChannels(c *fiber.Ctx) error {
	playlistID, err := optionalIntQuery(c, "playlist_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source", "The source id is invalid.", false)
	}
	groupID, err := optionalIntQuery(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	lineupID, err := optionalIntQuery(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	unusedOnly, err := optionalBoolQuery(c, "unused_only")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_filter", "The source usage filter is invalid.", false)
	}
	if unusedOnly && lineupID == nil {
		return v2Error(c, fiber.StatusBadRequest, "lineup_required", "Choose a lineup before filtering used sources.", false)
	}
	enabledOnly, err := optionalBoolQuery(c, "enabled_only")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_filter", "The source availability filter is invalid.", false)
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The page cursor is invalid or expired.", false)
	}
	items, total, err := database.Db.GetSourceChannels(c.UserContext(), playlistID, groupID, lineupID, unusedOnly, enabledOnly, strings.TrimSpace(c.Query("q")), limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "sources_unavailable", "Source channels could not be loaded.", true)
	}
	for index := range items {
		if items[index].LogoURL != nil && strings.TrimSpace(*items[index].LogoURL) != "" {
			items[index].LogoURL = scopedSourceLogoURL(items[index].ID)
		}
	}
	cursor, err := nextCursor(c, offset, len(items), total)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "cursor_unavailable", "The next page could not be created.", true)
	}
	return c.JSON(models.Paginated[models.SourceChannel]{Items: items, NextCursor: cursor, Total: total})
}

func V2StudioSourceChannelLogo(c *fiber.Ctx) error {
	sourceChannelID, err := parseID(c, "source_channel_id")
	if err != nil || sourceChannelID < 1 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source_channel", "The source channel id is invalid.", false)
	}
	rawURL, err := database.Db.GetSourceChannelLogoSource(c.UserContext(), sourceChannelID)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "source_logo_not_found", "That source logo was not found.", false)
	}
	name := logoassets.SourceLogoName(rawURL)
	path := filepath.Join(settings.LOGO_FILEPATH, name+".png")
	info, statErr := os.Stat(path)
	if errors.Is(statErr, os.ErrNotExist) {
		if storeErr := logoassets.StoreSourceLogo(c.UserContext(), rawURL, name); storeErr != nil {
			return v2Error(c, fiber.StatusBadGateway, "source_logo_unavailable", "The source logo could not be loaded.", true)
		}
	} else if statErr != nil || info.IsDir() || info.Size() == 0 {
		return v2Error(c, fiber.StatusBadGateway, "source_logo_unavailable", "The source logo could not be loaded.", true)
	}
	c.Set(fiber.HeaderContentType, "image/png")
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	return security.SendFileLiteral(c, path)
}

func V2SetStudioSourceChannelsEnabled(c *fiber.Ctx) error {
	request := models.SourceChannelEnableRequest{}
	if err := decodeStrict(c, &request); err != nil || len(request.ChannelIDs) == 0 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_selection", "Select at least one source channel.", false)
	}
	if err := database.Db.SetSourceChannelsEnabled(c.UserContext(), request.ChannelIDs, request.Enabled); err != nil {
		return v2Error(c, fiber.StatusUnprocessableEntity, "enablement_failed", "Source channel enablement could not be updated.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2StudioReview(c *fiber.Ctx) error {
	channelID, err := optionalIntQuery(c, "channel_id")
	if value := c.Params("channel_id"); value != "" {
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			err = parseErr
		} else {
			channelID = &parsed
		}
	}
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_cursor", "The page cursor is invalid or expired.", false)
	}
	items, total, err := database.Db.GetMatchReview(c.UserContext(), channelID, strings.TrimSpace(c.Query("q")), limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "review_unavailable", "Match review could not be loaded.", true)
	}
	for index := range items {
		if items[index].SourceLogoURL != nil && strings.TrimSpace(*items[index].SourceLogoURL) != "" {
			items[index].SourceLogoURL = scopedSourceLogoURL(items[index].SourceChannelID)
		}
	}
	cursor, err := nextCursor(c, offset, len(items), total)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "cursor_unavailable", "The next page could not be created.", true)
	}
	return c.JSON(models.Paginated[models.MatchReview]{Items: items, NextCursor: cursor, Total: total})
}

func V2StudioMatchRejections(c *fiber.Ctx) error {
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	items, err := database.Db.GetMatchRejections(c.UserContext(), channelID)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "rejections_unavailable", "Match history could not be loaded.", true)
	}
	for index := range items {
		if items[index].SourceChannelID != nil && items[index].LogoURL != nil && strings.TrimSpace(*items[index].LogoURL) != "" {
			items[index].LogoURL = scopedSourceLogoURL(*items[index].SourceChannelID)
		} else {
			items[index].LogoURL = nil
		}
	}
	return c.JSON(fiber.Map{"items": items, "next_cursor": nil, "total": len(items)})
}

func V2DeleteStudioMatchRejection(c *fiber.Ctx) error {
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	rejectionID, err := parseID(c, "rejection_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_rejection", "The rejection id is invalid.", false)
	}
	deleted, err := database.Db.DeleteMatchRejection(c.UserContext(), channelID, rejectionID)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "restore_failed", "The rejected source could not be restored.", true)
	}
	if !deleted {
		return v2Error(c, fiber.StatusNotFound, "rejection_not_found", "That rejected source was not found.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2StudioMatchSuggestions(c *fiber.Ctx) error {
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	limit, _ := strconv.Atoi(c.Query("limit", "5"))
	if limit < 1 {
		limit = 1
	}
	if limit > 5 {
		limit = 5
	}
	items, total, err := database.Db.GetMatchSuggestions(c.UserContext(), channelID, limit)
	if errors.Is(err, sql.ErrNoRows) {
		return v2Error(c, fiber.StatusNotFound, "channel_not_found", "That lineup channel was not found.", false)
	}
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "suggestions_unavailable", "Source suggestions could not be loaded.", true)
	}
	for index := range items {
		if items[index].LogoURL != nil && strings.TrimSpace(*items[index].LogoURL) != "" {
			items[index].LogoURL = scopedSourceLogoURL(items[index].SourceChannelID)
		}
	}
	return c.JSON(models.Paginated[models.MatchSuggestion]{Items: items, Total: total})
}

func V2AcceptStudioMatch(c *fiber.Ctx) error {
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	sourceChannelID, err := parseID(c, "source_channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source_channel", "The source channel id is invalid.", false)
	}
	if err := database.Db.AttachManualMatch(c.UserContext(), channelID, sourceChannelID); errors.Is(err, sql.ErrNoRows) {
		return v2Error(c, fiber.StatusNotFound, "match_target_not_found", "That lineup channel or source channel was not found.", false)
	} else if err != nil {
		return v2Error(c, fiber.StatusUnprocessableEntity, "match_conflict", "That source could not be attached as a backup.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2MoveStudioMatch(c *fiber.Ctx) error {
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	sourceChannelID, err := parseID(c, "source_channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source_channel", "The source channel id is invalid.", false)
	}
	body := positionRequest{}
	if err := decodeStrict(c, &body); err != nil || (body.BeforeID == nil) == (body.AfterID == nil) {
		return v2Error(c, fiber.StatusBadRequest, "invalid_move", "Choose exactly one source to move before or after.", false)
	}
	if err := database.Db.MoveMatchVariant(c.UserContext(), channelID, sourceChannelID, body.BeforeID, body.AfterID); errors.Is(err, sql.ErrNoRows) {
		return v2Error(c, fiber.StatusNotFound, "match_not_found", "That source variant or its destination was not found.", false)
	} else if err != nil {
		log.Error().Err(err).Msg("Source variant move failed")
		return v2Error(c, fiber.StatusUnprocessableEntity, "move_failed", "The source variant could not be moved.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2RejectStudioMatch(c *fiber.Ctx) error {
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	sourceChannelID, err := parseID(c, "source_channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source_channel", "The source channel id is invalid.", false)
	}
	item := &models.TemplateChannelItem{ChannelId: channelID, PlaylistChannelId: sourceChannelID}
	if err := database.Db.DeleteTmplChannelItem(item); err != nil {
		return v2Error(c, fiber.StatusNotFound, "match_not_found", "That source match was not found.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2MoveStudioChannel(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	if linked, linkErr := database.Db.IsSourceLinkedGroup(c.UserContext(), groupID); linkErr != nil {
		return v2Error(c, fiber.StatusInternalServerError, "group_state_unavailable", "The group state could not be checked.", true)
	} else if linked {
		return v2Error(c, fiber.StatusConflict, "synced_group_managed", "Channel order is controlled by the connected source group.", false)
	}
	body := positionRequest{}
	if err := decodeStrict(c, &body); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_request", "The move request is invalid.", false)
	}
	if err := database.Db.MoveWorkspaceChannel(c.UserContext(), groupID, channelID, body.BeforeID, body.AfterID); err != nil {
		log.Error().Err(err).Msg("Lineup channel move failed")
		return v2Error(c, fiber.StatusUnprocessableEntity, "move_failed", "The lineup channel could not be moved.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2BatchAddStudioChannels(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	if linked, linkErr := database.Db.IsSourceLinkedGroup(c.UserContext(), groupID); linkErr != nil {
		return v2Error(c, fiber.StatusInternalServerError, "group_state_unavailable", "The group state could not be checked.", true)
	} else if linked {
		return v2Error(c, fiber.StatusConflict, "synced_group_managed", "Channels are added by the connected source group.", false)
	}
	request := models.WorkspaceBatchAddRequest{}
	if err := decodeStrict(c, &request); err != nil || len(request.SourceChannelIDs) == 0 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_selection", "Select at least one source channel.", false)
	}
	uuids := make([]string, len(request.SourceChannelIDs))
	for index := range uuids {
		uuids[index] = utils.CreateUuid()
	}
	channelIDs, err := database.Db.BatchAddWorkspaceChannels(c.UserContext(), groupID, request.SourceChannelIDs, uuids)
	if err != nil {
		return v2Error(c, fiber.StatusUnprocessableEntity, "batch_add_failed", "The selected source channels could not be added.", false)
	}
	matchImportedChannels(channelIDs)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"channel_ids": channelIDs, "total": len(channelIDs)})
}

func V2BatchRemoveStudioChannels(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	if linked, linkErr := database.Db.IsSourceLinkedGroup(c.UserContext(), groupID); linkErr != nil {
		return v2Error(c, fiber.StatusInternalServerError, "group_state_unavailable", "The group state could not be checked.", true)
	} else if linked {
		return v2Error(c, fiber.StatusConflict, "synced_group_managed", "Channels are removed by the connected source group.", false)
	}
	request := models.WorkspaceBatchRemoveRequest{}
	if err := decodeStrict(c, &request); err != nil || len(request.ChannelIDs) == 0 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_selection", "Select at least one lineup channel.", false)
	}
	if err := database.Db.BatchRemoveWorkspaceChannels(c.UserContext(), groupID, request.ChannelIDs); err != nil {
		return v2Error(c, fiber.StatusUnprocessableEntity, "batch_remove_failed", "The selected channels could not be removed.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2BatchMoveStudioChannels(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	request := models.WorkspaceBatchMoveRequest{}
	if err := decodeStrict(c, &request); err != nil || len(request.ChannelIDs) == 0 || request.TargetGroupID < 1 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_move", "Select channels and a destination group.", false)
	}
	for _, candidateGroupID := range []int64{groupID, request.TargetGroupID} {
		if linked, linkErr := database.Db.IsSourceLinkedGroup(c.UserContext(), candidateGroupID); linkErr != nil {
			return v2Error(c, fiber.StatusInternalServerError, "group_state_unavailable", "The group state could not be checked.", true)
		} else if linked {
			return v2Error(c, fiber.StatusConflict, "synced_group_managed", "Channels cannot be moved into or out of a synced group.", false)
		}
	}
	if err := database.Db.BatchMoveWorkspaceChannels(c.UserContext(), groupID, request.TargetGroupID, request.ChannelIDs, request.BeforeID, request.AfterID); err != nil {
		log.Error().Err(err).Msg("Lineup channel batch move failed")
		return v2Error(c, fiber.StatusUnprocessableEntity, "batch_move_failed", "The selected lineup channels could not be moved.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2UpdateStudioChannel(c *fiber.Ctx) error {
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	update := models.WorkspaceChannelUpdate{}
	if err := decodeStrict(c, &update); err != nil || strings.TrimSpace(update.Name) == "" {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "A channel name is required.", false)
	}
	if err := database.Db.UpdateWorkspaceChannel(c.UserContext(), channelID, update); err != nil {
		return v2Error(c, fiber.StatusUnprocessableEntity, "channel_update_failed", "The channel could not be updated.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type jobProgressFunc func(progress int, message string)

type v2JobProgressReporter struct {
	mu           sync.Mutex
	jobID        int64
	lastProgress int
	lastMessage  string
	lastWrite    time.Time
}

func (r *v2JobProgressReporter) report(progress int, message string, force bool) {
	if progress < 0 {
		progress = 0
	}
	if progress > 99 {
		progress = 99
	}
	message = strings.TrimSpace(message)

	r.mu.Lock()
	defer r.mu.Unlock()
	if progress < r.lastProgress || (progress == r.lastProgress && message == r.lastMessage) {
		return
	}
	now := time.Now()
	if !force && !r.lastWrite.IsZero() && now.Sub(r.lastWrite) < 500*time.Millisecond {
		return
	}
	r.lastProgress = progress
	r.lastMessage = message
	r.lastWrite = now
	persistV2JobUpdate(context.Background(), r.jobID, "running", progress, message, "")
}

func (r *v2JobProgressReporter) Report(progress int, message string) {
	r.report(progress, message, false)
}

func launchV2Job(ctx context.Context, kind, resource string, resourceID int64, queuedMessage, runningMessage, successMessage string, work func(jobProgressFunc) error) (*models.OperationJob, error) {
	job, err := database.Db.CreateJob(ctx, kind, resource, &resourceID, queuedMessage)
	if err != nil {
		return nil, err
	}
	go func(jobID int64) {
		ctx := context.Background()
		reporter := &v2JobProgressReporter{jobID: jobID}
		reporter.report(0, runningMessage, true)
		defer func() {
			if recovered := recover(); recovered != nil {
				persistV2JobUpdate(ctx, jobID, "failed", 100, "The operation stopped unexpectedly.", "worker_panic")
			}
		}()
		if workErr := work(reporter.Report); workErr != nil {
			message := sanitizeOperationError(workErr)
			errorCode := fmt.Sprintf("%s_%s_failed", resource, kind)
			persistV2JobUpdate(ctx, jobID, "failed", 100, message, errorCode)
			return
		}
		persistV2JobUpdate(ctx, jobID, "succeeded", 100, successMessage, "")
	}(job.ID)
	return job, nil
}

func sanitizeOperationError(err error) string {
	if err == nil {
		return "The operation failed."
	}
	message := strings.TrimSpace(streaming.SanitizeDiagnostic(err.Error()))
	lower := strings.ToLower(message)
	for _, marker := range []string{"select ", "insert ", "update ", "delete ", "sql", "constraint failed", "no such table", "syntax error", "stack trace", "/xivi/", "/workspaces/", `:\`} {
		if strings.Contains(lower, marker) {
			return "The operation failed while processing protected server data."
		}
	}
	if message == "" {
		return "The operation failed."
	}
	return truncateRunes(message, 500)
}

func persistV2JobUpdate(ctx context.Context, jobID int64, status string, progress int, message, errorCode string) {
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if err = database.Db.UpdateJob(ctx, jobID, status, progress, message, errorCode); err == nil {
			return
		}
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
		}
	}
	log.Error().Err(err).Int64("job_id", jobID).Str("status", status).Msg("Failed to persist operation job state")
}

func queueV2Job(c *fiber.Ctx, kind, resource string, resourceID int64, queuedMessage, runningMessage, successMessage string, work func(jobProgressFunc) error) error {
	job, err := launchV2Job(c.UserContext(), kind, resource, resourceID, queuedMessage, runningMessage, successMessage, work)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "job_unavailable", "The operation could not be queued.", true)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"job_id": job.ID, "status": job.Status})
}

func V2RefreshSource(c *fiber.Ctx) error {
	id, err := parseID(c, "source_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source", "The source id is invalid.", false)
	}
	playlist, err := database.Db.GetPlaylist(id)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "source_not_found", "That playlist source was not found.", false)
	}
	return queueV2Job(c, "refresh", "playlist", id, "Playlist refresh queued.", "Importing channels and syncing connected groups…", "Playlist refresh complete.", func(report jobProgressFunc) error {
		startedAt := time.Now()
		parser := utils.M3uParser{Progress: utils.ProgressReporter(report)}
		if err := parser.ParseM3u(*playlist); err != nil {
			return fmt.Errorf("playlist refresh failed: %w", err)
		}
		report(86, "Removing channels no longer present in the source…")
		cron.CleanPlaylist(*playlist, startedAt)
		report(92, "Synchronizing connected lineup groups…")
		results, err := database.Db.SyncSourceGroupsForPlaylist(context.Background(), playlist.ID)
		if err == nil {
			report(98, fmt.Sprintf("Finalizing playlist refresh after syncing %d groups…", len(results)))
		}
		return err
	})
}

func V2RefreshGuideSource(c *fiber.Ctx) error {
	id, err := parseID(c, "source_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_guide_source", "The guide source id is invalid.", false)
	}
	epg, err := database.Db.GetEpg(c.UserContext(), id)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "guide_source_not_found", "That guide source was not found.", false)
	}
	return queueV2Job(c, "refresh", "epg", id, "Guide refresh queued.", "Importing XMLTV schedule…", "Guide refresh complete.", func(report jobProgressFunc) error {
		if err := utils.ParseEpgWithProgress(epg, utils.ProgressReporter(report)); err != nil {
			return fmt.Errorf("guide refresh failed: %w", err)
		}
		report(97, "Removing expired guide programmes…")
		cron.CleanupOldEpgProgrammes()
		report(99, "Finalizing guide refresh…")
		return nil
	})
}

func V2PublishLineup(c *fiber.Ctx) error {
	id, err := parseID(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	lineup, err := database.Db.GetTemplate(id)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "lineup_not_found", "That lineup was not found.", false)
	}
	return queueV2Job(c, "publish", "lineup", id, "Publication queued.", "Building M3U and XMLTV outputs…", "Lineup published.", func(report jobProgressFunc) error {
		report(15, "Building the ordered M3U output…")
		if err := utils.NewM3uTools().CreateM3u(*lineup); err != nil {
			return fmt.Errorf("M3U publication failed: %w", err)
		}
		report(45, "Building the XMLTV output…")
		xmltvProgress := func(progress int, message string) {
			mappedProgress := 45 + progress*52/100
			if mappedProgress > 97 {
				mappedProgress = 97
			}
			report(mappedProgress, message)
		}
		if err := utils.CreateEpgXMLWithProgress(*lineup, xmltvProgress); err != nil {
			return fmt.Errorf("XMLTV publication failed: %w", err)
		}
		report(99, "Finalizing published outputs…")
		return nil
	})
}

func V2Jobs(c *fiber.Ctx) error {
	items, err := database.Db.GetJobs(c.UserContext(), 30)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "jobs_unavailable", "Jobs could not be loaded.", true)
	}
	return c.JSON(fiber.Map{"items": items, "next_cursor": nil, "total": len(items)})
}

func V2Job(c *fiber.Ctx) error {
	id, err := parseID(c, "job_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_job", "The job id is invalid.", false)
	}
	item, err := database.Db.GetJob(c.UserContext(), id)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "job_not_found", "That job was not found.", false)
	}
	return c.JSON(item)
}

// V2Events is a resumable, lightweight SSE feed. Resource snapshots make it
// useful before individual refresh/import workers are migrated to jobs.
func V2Events(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		write := func() (bool, error) {
			jobs, err := database.Db.GetJobs(context.Background(), 10)
			if err != nil {
				return false, err
			}
			streamSummary := streaming.DefaultManager.Summary()
			payload, _ := json.Marshal(fiber.Map{"jobs": jobs, "streams": streamSummary, "at": time.Now().UTC()})
			lastID := int64(0)
			active := false
			if len(jobs) > 0 {
				lastID = jobs[0].ID
			}
			for _, job := range jobs {
				if job.Status == "queued" || job.Status == "running" {
					active = true
					break
				}
			}
			if streamSummary.Sessions > 0 {
				active = true
			}
			if _, err := fmt.Fprintf(w, "id: %d\nretry: 5000\nevent: snapshot\ndata: %s\n\n", lastID, payload); err != nil {
				return false, err
			}
			return active, w.Flush()
		}
		for {
			active, err := write()
			if err != nil {
				return
			}
			delay := 15 * time.Second
			if active {
				delay = 2 * time.Second
			}
			timer := time.NewTimer(delay)
			<-timer.C
		}
	})
	return nil
}

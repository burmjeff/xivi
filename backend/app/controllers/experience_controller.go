package controllers

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/app/queries"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/cron"
	"xivi/backend/platform/database"

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

func pageParams(c *fiber.Ctx) (int, int) {
	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	offset := 0
	if cursor := c.Query("cursor"); cursor != "" {
		if decoded, err := base64.RawURLEncoding.DecodeString(cursor); err == nil {
			offset, _ = strconv.Atoi(string(decoded))
		}
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func nextCursor(offset, count int, total int64) *string {
	next := offset + count
	if int64(next) >= total {
		return nil
	}
	value := base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(next)))
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
	items, err := database.Db.GetLineupSummaries(c.UserContext())
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "lineups_unavailable", "Lineups could not be loaded.", true)
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
	limit, offset := pageParams(c)
	items, total, err := database.Db.GetGuideChannels(c.UserContext(), lineupID, groupID, strings.TrimSpace(c.Query("q")), from, to, limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "channels_unavailable", "Channels could not be loaded.", true)
	}
	return c.JSON(models.Paginated[models.GuideChannel]{Items: items, NextCursor: nextCursor(offset, len(items), total), Total: total})
}

func V2WatchChannels(c *fiber.Ctx) error { return v2LineupChannels(c, false) }
func V2WatchGuide(c *fiber.Ctx) error    { return v2LineupChannels(c, true) }

func V2WatchChannel(c *fiber.Ctx) error {
	id, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	now := time.Now().UTC()
	item, err := database.Db.GetGuideChannel(c.UserContext(), id, now.Add(-2*time.Hour), now.Add(12*time.Hour))
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "channel_not_found", "That channel is no longer available.", false)
	}
	return c.JSON(item)
}

func V2Search(c *fiber.Ctx) error {
	lineupID, err := optionalIntQuery(c, "lineup_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	if lineupID == nil {
		lineups, loadErr := database.Db.GetLineupSummaries(c.UserContext())
		if loadErr != nil || len(lineups) == 0 {
			return c.JSON(fiber.Map{"items": []any{}, "next_cursor": nil, "total": 0})
		}
		lineupID = &lineups[0].ID
	}
	limit, offset := pageParams(c)
	now := time.Now().UTC()
	items, total, err := database.Db.GetGuideChannels(c.UserContext(), *lineupID, nil, strings.TrimSpace(c.Query("q")), now.Add(-time.Hour), now.Add(4*time.Hour), limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "search_unavailable", "Search is temporarily unavailable.", true)
	}
	return c.JSON(models.Paginated[models.GuideChannel]{Items: items, NextCursor: nextCursor(offset, len(items), total), Total: total})
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
	if err := c.BodyParser(&body); err != nil || (body.BeforeID == nil) == (body.AfterID == nil) {
		return v2Error(c, fiber.StatusBadRequest, "invalid_move", "Choose exactly one group to move before or after.", false)
	}
	if err := database.Db.MoveWorkspaceGroup(c.UserContext(), lineupID, groupID, body.BeforeID, body.AfterID); err != nil {
		if errors.Is(err, queries.ErrLineupNotFound) || errors.Is(err, queries.ErrStudioGroupNotFound) {
			return studioGroupError(c, err)
		}
		return v2Error(c, fiber.StatusUnprocessableEntity, "group_move_failed", err.Error(), false)
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
	if err := c.BodyParser(&request); err != nil {
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
	job, err := launchV2Job(c.UserContext(), "sync", "group", groupID, "Group sync queued.", "Reconciling the source group…", "Synced group updated.", func() error {
		_, err := database.Db.SyncSourceGroup(context.Background(), groupID)
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
	if err := c.BodyParser(&request); err != nil {
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
	limit, offset := pageParams(c)
	items, total, err := database.Db.GetSourceGroups(c.UserContext(), playlistID, strings.TrimSpace(c.Query("q")), limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "source_groups_unavailable", "Source groups could not be loaded.", true)
	}
	return c.JSON(models.Paginated[models.SourceGroup]{Items: items, NextCursor: nextCursor(offset, len(items), total), Total: total})
}

func V2SetStudioGroupSourceLink(c *fiber.Ctx) error {
	groupID, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	request := models.SourceGroupLinkRequest{}
	if err := c.BodyParser(&request); err != nil || request.SourceGroupID < 1 {
		return v2Error(c, fiber.StatusBadRequest, "invalid_source_group", "Choose a source group to sync.", false)
	}
	if err := database.Db.SetSourceGroupLink(c.UserContext(), groupID, request); err != nil {
		return studioGroupError(c, err)
	}
	return queueV2Job(c, "sync", "group", groupID, "Group sync queued.", "Reconciling the source group…", "Synced group updated.", func() error {
		_, err := database.Db.SyncSourceGroup(context.Background(), groupID)
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
	return queueV2Job(c, "sync", "group", groupID, "Group sync queued.", "Reconciling the source group…", "Synced group updated.", func() error {
		_, err := database.Db.SyncSourceGroup(context.Background(), groupID)
		return err
	})
}

func V2StudioGroupChannels(c *fiber.Ctx) error {
	id, err := parseID(c, "group_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_group", "The group id is invalid.", false)
	}
	matchHealth := strings.TrimSpace(c.Query("match"))
	if matchHealth != "" && matchHealth != "unmatched" && matchHealth != "low-confidence" {
		return v2Error(c, fiber.StatusBadRequest, "invalid_match_filter", "Choose unmatched or low-confidence match health.", false)
	}
	limit, offset := pageParams(c)
	items, total, err := database.Db.GetWorkspaceChannels(c.UserContext(), id, strings.TrimSpace(c.Query("q")), matchHealth, limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "workspace_unavailable", "Lineup channels could not be loaded.", true)
	}
	return c.JSON(models.Paginated[models.WorkspaceChannel]{Items: items, NextCursor: nextCursor(offset, len(items), total), Total: total})
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
	limit, offset := pageParams(c)
	items, total, err := database.Db.GetSourceChannels(c.UserContext(), playlistID, groupID, strings.TrimSpace(c.Query("q")), limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "sources_unavailable", "Source channels could not be loaded.", true)
	}
	return c.JSON(models.Paginated[models.SourceChannel]{Items: items, NextCursor: nextCursor(offset, len(items), total), Total: total})
}

func V2SetStudioSourceChannelsEnabled(c *fiber.Ctx) error {
	request := models.SourceChannelEnableRequest{}
	if err := c.BodyParser(&request); err != nil || len(request.ChannelIDs) == 0 {
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
	limit, offset := pageParams(c)
	items, total, err := database.Db.GetMatchReview(c.UserContext(), channelID, strings.TrimSpace(c.Query("q")), limit, offset)
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "review_unavailable", "Match review could not be loaded.", true)
	}
	return c.JSON(models.Paginated[models.MatchReview]{Items: items, NextCursor: nextCursor(offset, len(items), total), Total: total})
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
	items, err := database.Db.GetMatchSuggestions(c.UserContext(), channelID, limit)
	if errors.Is(err, sql.ErrNoRows) {
		return v2Error(c, fiber.StatusNotFound, "channel_not_found", "That lineup channel was not found.", false)
	}
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "suggestions_unavailable", "Source suggestions could not be loaded.", true)
	}
	return c.JSON(models.Paginated[models.MatchSuggestion]{Items: items, Total: int64(len(items))})
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
	if _, err := database.Db.GetPlChannel(sourceChannelID); err != nil {
		return v2Error(c, fiber.StatusNotFound, "source_channel_not_found", "That source channel was not found.", false)
	}
	item := &models.TemplateChannelItem{ChannelId: channelID, PlaylistChannelId: sourceChannelID, MatchMethod: "manual", MatcherVersion: 2, ManualLocked: true}
	if existing, _ := database.Db.GetTmplChannelItem(item); existing == nil {
		if _, err := database.Db.CreateTmplChannelItem(item); err != nil {
			return v2Error(c, fiber.StatusUnprocessableEntity, "match_conflict", "That source could not be attached. A source from the same playlist may already be locked.", false)
		}
	}
	if err := database.Db.ClearChannelMatchRejection(channelID, sourceChannelID); err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "match_accept_failed", "The match was attached but its rejection history could not be cleared.", true)
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
	if err := c.BodyParser(&body); err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_request", "The move request is invalid.", false)
	}
	if err := database.Db.MoveWorkspaceChannel(c.UserContext(), groupID, channelID, body.BeforeID, body.AfterID); err != nil {
		return v2Error(c, fiber.StatusUnprocessableEntity, "move_failed", err.Error(), false)
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
	if err := c.BodyParser(&request); err != nil || len(request.SourceChannelIDs) == 0 {
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
	if err := c.BodyParser(&request); err != nil || len(request.ChannelIDs) == 0 {
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
	if err := c.BodyParser(&request); err != nil || len(request.ChannelIDs) == 0 || request.TargetGroupID < 1 {
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
		return v2Error(c, fiber.StatusUnprocessableEntity, "batch_move_failed", err.Error(), false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func V2UpdateStudioChannel(c *fiber.Ctx) error {
	channelID, err := parseID(c, "channel_id")
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "The channel id is invalid.", false)
	}
	update := models.WorkspaceChannelUpdate{}
	if err := c.BodyParser(&update); err != nil || strings.TrimSpace(update.Name) == "" {
		return v2Error(c, fiber.StatusBadRequest, "invalid_channel", "A channel name is required.", false)
	}
	if err := database.Db.UpdateWorkspaceChannel(c.UserContext(), channelID, update); err != nil {
		return v2Error(c, fiber.StatusUnprocessableEntity, "channel_update_failed", "The channel could not be updated.", false)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func launchV2Job(ctx context.Context, kind, resource string, resourceID int64, queuedMessage, runningMessage, successMessage string, work func() error) (*models.OperationJob, error) {
	job, err := database.Db.CreateJob(ctx, kind, resource, &resourceID, queuedMessage)
	if err != nil {
		return nil, err
	}
	go func(jobID int64) {
		ctx := context.Background()
		_ = database.Db.UpdateJob(ctx, jobID, "running", 10, runningMessage, "")
		defer func() {
			if recovered := recover(); recovered != nil {
				_ = database.Db.UpdateJob(ctx, jobID, "failed", 100, "The operation stopped unexpectedly.", "worker_panic")
			}
		}()
		if workErr := work(); workErr != nil {
			message := strings.TrimSpace(workErr.Error())
			if message == "" {
				message = "The operation failed."
			}
			errorCode := fmt.Sprintf("%s_%s_failed", resource, kind)
			_ = database.Db.UpdateJob(ctx, jobID, "failed", 100, message, errorCode)
			return
		}
		_ = database.Db.UpdateJob(ctx, jobID, "succeeded", 100, successMessage, "")
	}(job.ID)
	return job, nil
}

func queueV2Job(c *fiber.Ctx, kind, resource string, resourceID int64, queuedMessage, runningMessage, successMessage string, work func() error) error {
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
	return queueV2Job(c, "refresh", "playlist", id, "Playlist refresh queued.", "Importing channels and syncing connected groups…", "Playlist refresh complete.", func() error {
		startedAt := time.Now()
		parser := utils.M3uParser{}
		if err := parser.ParseM3u(*playlist); err != nil {
			return fmt.Errorf("playlist refresh failed: %w", err)
		}
		cron.CleanPlaylist(*playlist, startedAt)
		_, err := database.Db.SyncSourceGroupsForPlaylist(context.Background(), playlist.ID)
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
	return queueV2Job(c, "refresh", "epg", id, "Guide refresh queued.", "Importing XMLTV schedule…", "Guide refresh complete.", func() error {
		if err := utils.ParseEpg(epg); err != nil {
			return fmt.Errorf("guide refresh failed: %w", err)
		}
		cron.CleanupOldEpgProgrammes()
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
	return queueV2Job(c, "publish", "lineup", id, "Publication queued.", "Building M3U and XMLTV outputs…", "Lineup published.", func() error {
		if err := utils.NewM3uTools().CreateM3u(*lineup); err != nil {
			return fmt.Errorf("M3U publication failed: %w", err)
		}
		if err := utils.CreateEpgXML(*lineup); err != nil {
			return fmt.Errorf("XMLTV publication failed: %w", err)
		}
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
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		write := func() error {
			jobs, err := database.Db.GetJobs(context.Background(), 10)
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(fiber.Map{"jobs": jobs, "at": time.Now().UTC()})
			lastID := int64(0)
			if len(jobs) > 0 {
				lastID = jobs[0].ID
			}
			if _, err := fmt.Fprintf(w, "id: %d\nretry: 5000\nevent: snapshot\ndata: %s\n\n", lastID, payload); err != nil {
				return err
			}
			return w.Flush()
		}
		if write() != nil {
			return
		}
		for range ticker.C {
			if write() != nil {
				return
			}
		}
	})
	return nil
}

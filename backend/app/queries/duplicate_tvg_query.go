package queries

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"xivi/backend/app/models"
	"xivi/backend/pkg/channelmatch"

	"github.com/jmoiron/sqlx"
)

var (
	ErrDuplicateTVGIDNotFound = errors.New("duplicate TVG ID review was not found")
	ErrDuplicateTVGIDManaged  = errors.New("source-managed duplicate channels cannot be merged")
	ErrDuplicateMergeTarget   = errors.New("merge target is not part of the duplicate TVG ID review")
)

type duplicateTVGMembership struct {
	LineupID    int64  `db:"lineup_id"`
	LineupName  string `db:"lineup_name"`
	ChannelID   int64  `db:"channel_id"`
	ChannelName string `db:"channel_name"`
	TVGID       string `db:"tvgid"`
	UUID        string `db:"uuid"`
	GroupID     int64  `db:"group_id"`
	GroupName   string `db:"group_name"`
	SourceCount int64  `db:"source_count"`
	SourceNames string `db:"source_names"`
	Managed     bool   `db:"managed"`
}

type duplicateTVGAcknowledgement struct {
	LineupID         int64  `db:"lineup_id"`
	TVGID            string `db:"tvg_id_norm"`
	ChannelSignature string `db:"channel_signature"`
}

type duplicateTVGBuilder struct {
	review   models.DuplicateTVGIDReview
	channels map[int64]*models.DuplicateTVGIDChannel
}

func duplicateTVGKey(lineupID int64, tvgID string) string {
	return strconv.FormatInt(lineupID, 10) + "\x00" + tvgID
}

func duplicateTVGSignature(channels []models.DuplicateTVGIDChannel) string {
	ids := make([]int64, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, len(ids))
	for index, id := range ids {
		parts[index] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

func appendUniqueInt64(values []int64, value int64) []int64 {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (q *ExperienceQueries) loadDuplicateTVGIDReviews(ctx context.Context, lineupID *int64, includeAcknowledged bool) ([]models.DuplicateTVGIDReview, error) {
	rows := []duplicateTVGMembership{}
	if err := q.SelectContext(ctx, &rows, `WITH source_counts AS (
			SELECT channel_id, COUNT(*) AS source_count FROM templatechannelitem GROUP BY channel_id
		), ordered_source_names AS (
			SELECT tci.channel_id,
				p.name || ' · ' || COALESCE(NULLIF(pc.tvg_name, ''), NULLIF(pc.title, ''), 'Source ' || pc.id) AS source_name
			FROM templatechannelitem tci
			JOIN playlistchannel pc ON pc.id = tci.playlist_channel_id
			JOIN playlistgroup pg ON pg.id = pc.group_id
			JOIN playlist p ON p.id = pg.playlist_id
			ORDER BY tci.channel_id, tci.orderr, tci.id
		), source_names AS (
			SELECT channel_id, GROUP_CONCAT(source_name, CHAR(31)) AS source_names
			FROM ordered_source_names GROUP BY channel_id
		), managed_channels AS (
			SELECT DISTINCT template_channel_id AS channel_id FROM lineup_group_source_member
		)
		SELECT t.id AS lineup_id, t.name AS lineup_name,
			tc.id AS channel_id, tc.name AS channel_name, tc.tvgid, tc.uuid,
			tg.id AS group_id, tg.name AS group_name,
			COALESCE(sc.source_count, 0) AS source_count,
			COALESCE(sn.source_names, '') AS source_names,
			CASE WHEN mc.channel_id IS NULL THEN false ELSE true END AS managed
		FROM template t
		JOIN template_group_item tgi ON tgi.template_id = t.id
		JOIN templategroup tg ON tg.id = tgi.group_id
		JOIN template_group_channel tgc ON tgc.group_id = tg.id
		JOIN templatechannel tc ON tc.id = tgc.channel_id
		LEFT JOIN source_counts sc ON sc.channel_id = tc.id
		LEFT JOIN source_names sn ON sn.channel_id = tc.id
		LEFT JOIN managed_channels mc ON mc.channel_id = tc.id
		WHERE NULLIF(TRIM(tc.tvgid), '') IS NOT NULL
		ORDER BY t.id, tgi.orderr, tgc.orderr, tc.id`); err != nil {
		return nil, err
	}

	acknowledgements := []duplicateTVGAcknowledgement{}
	if err := q.SelectContext(ctx, &acknowledgements, `SELECT lineup_id, tvg_id_norm, channel_signature FROM lineup_tvgid_review_ack`); err != nil {
		return nil, err
	}
	acknowledged := make(map[string]string, len(acknowledgements))
	for _, item := range acknowledgements {
		acknowledged[duplicateTVGKey(item.LineupID, item.TVGID)] = item.ChannelSignature
	}

	lineupsByChannel := map[int64]map[int64]string{}
	builders := map[string]*duplicateTVGBuilder{}
	for _, row := range rows {
		if lineupsByChannel[row.ChannelID] == nil {
			lineupsByChannel[row.ChannelID] = map[int64]string{}
		}
		lineupsByChannel[row.ChannelID][row.LineupID] = row.LineupName

		normalized := channelmatch.NormalizeTvgID(row.TVGID)
		if normalized == "" {
			continue
		}
		key := duplicateTVGKey(row.LineupID, normalized)
		builder := builders[key]
		if builder == nil {
			builder = &duplicateTVGBuilder{
				review: models.DuplicateTVGIDReview{
					LineupID: row.LineupID, LineupName: row.LineupName, TVGID: strings.TrimSpace(row.TVGID), MergeAllowed: true,
				},
				channels: map[int64]*models.DuplicateTVGIDChannel{},
			}
			builders[key] = builder
		}
		channel := builder.channels[row.ChannelID]
		if channel == nil {
			channel = &models.DuplicateTVGIDChannel{
				ID: row.ChannelID, Name: row.ChannelName, UUID: row.UUID,
				SourceCount: row.SourceCount, Managed: row.Managed,
			}
			if row.SourceNames != "" {
				channel.SourceNames = strings.Split(row.SourceNames, "\x1f")
			} else {
				channel.SourceNames = []string{}
			}
			builder.channels[row.ChannelID] = channel
		}
		channel.GroupIDs = appendUniqueInt64(channel.GroupIDs, row.GroupID)
		channel.GroupNames = appendUniqueString(channel.GroupNames, row.GroupName)
		if row.Managed {
			builder.review.MergeAllowed = false
		}
	}

	reviews := []models.DuplicateTVGIDReview{}
	for key, builder := range builders {
		if len(builder.channels) < 2 || (lineupID != nil && builder.review.LineupID != *lineupID) {
			continue
		}
		builder.review.Channels = make([]models.DuplicateTVGIDChannel, 0, len(builder.channels))
		lineupNames := map[int64]string{}
		for _, channel := range builder.channels {
			sort.Slice(channel.GroupIDs, func(i, j int) bool { return channel.GroupIDs[i] < channel.GroupIDs[j] })
			sort.Strings(channel.GroupNames)
			builder.review.Channels = append(builder.review.Channels, *channel)
			for id, name := range lineupsByChannel[channel.ID] {
				lineupNames[id] = name
			}
		}
		sort.Slice(builder.review.Channels, func(i, j int) bool {
			if builder.review.Channels[i].Name == builder.review.Channels[j].Name {
				return builder.review.Channels[i].ID < builder.review.Channels[j].ID
			}
			return strings.ToLower(builder.review.Channels[i].Name) < strings.ToLower(builder.review.Channels[j].Name)
		})
		lineupIDs := make([]int64, 0, len(lineupNames))
		for id := range lineupNames {
			lineupIDs = append(lineupIDs, id)
		}
		sort.Slice(lineupIDs, func(i, j int) bool { return lineupIDs[i] < lineupIDs[j] })
		for _, id := range lineupIDs {
			builder.review.AffectedLineups = append(builder.review.AffectedLineups, models.DuplicateTVGIDLineup{ID: id, Name: lineupNames[id]})
		}
		builder.review.Acknowledged = acknowledged[key] == duplicateTVGSignature(builder.review.Channels)
		if includeAcknowledged || !builder.review.Acknowledged {
			reviews = append(reviews, builder.review)
		}
	}
	sort.Slice(reviews, func(i, j int) bool {
		if reviews[i].LineupID == reviews[j].LineupID {
			return strings.ToLower(reviews[i].TVGID) < strings.ToLower(reviews[j].TVGID)
		}
		return reviews[i].LineupID < reviews[j].LineupID
	})
	return reviews, nil
}

func (q *ExperienceQueries) GetDuplicateTVGIDReviews(ctx context.Context, lineupID int64, includeAcknowledged bool, limit, offset int) ([]models.DuplicateTVGIDReview, int64, error) {
	reviews, err := q.loadDuplicateTVGIDReviews(ctx, &lineupID, includeAcknowledged)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(reviews))
	if offset >= len(reviews) {
		return []models.DuplicateTVGIDReview{}, total, nil
	}
	end := min(len(reviews), offset+limit)
	return reviews[offset:end], total, nil
}

func (q *ExperienceQueries) SetDuplicateTVGIDReviewAcknowledged(ctx context.Context, lineupID int64, tvgID string, acknowledged bool) error {
	normalized := channelmatch.NormalizeTvgID(tvgID)
	if normalized == "" {
		return ErrDuplicateTVGIDNotFound
	}
	reviews, err := q.loadDuplicateTVGIDReviews(ctx, nil, true)
	if err != nil {
		return err
	}
	targetSignature := ""
	for _, review := range reviews {
		if review.LineupID == lineupID && channelmatch.NormalizeTvgID(review.TVGID) == normalized {
			targetSignature = duplicateTVGSignature(review.Channels)
			break
		}
	}
	if targetSignature == "" {
		return ErrDuplicateTVGIDNotFound
	}
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		for _, review := range reviews {
			if channelmatch.NormalizeTvgID(review.TVGID) != normalized || duplicateTVGSignature(review.Channels) != targetSignature {
				continue
			}
			if !acknowledged {
				if _, err := tx.ExecContext(ctx, `DELETE FROM lineup_tvgid_review_ack WHERE lineup_id = ? AND tvg_id_norm = ?`, review.LineupID, normalized); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO lineup_tvgid_review_ack (lineup_id, tvg_id_norm, channel_signature)
				VALUES (?, ?, ?) ON CONFLICT(lineup_id, tvg_id_norm) DO UPDATE SET
				channel_signature = excluded.channel_signature, created_at = CURRENT_TIMESTAMP`,
				review.LineupID, normalized, targetSignature); err != nil {
				return err
			}
		}
		return nil
	})
}

type duplicateMergeSource struct {
	PlaylistChannelID int64 `db:"playlist_channel_id"`
}

func (q *ExperienceQueries) MergeDuplicateTVGIDChannels(ctx context.Context, lineupID int64, tvgID string, keepChannelID int64) (*models.DuplicateTVGIDMergeResult, error) {
	normalized := channelmatch.NormalizeTvgID(tvgID)
	reviews, err := q.loadDuplicateTVGIDReviews(ctx, &lineupID, true)
	if err != nil {
		return nil, err
	}
	var review *models.DuplicateTVGIDReview
	for index := range reviews {
		if channelmatch.NormalizeTvgID(reviews[index].TVGID) == normalized {
			review = &reviews[index]
			break
		}
	}
	if review == nil {
		return nil, ErrDuplicateTVGIDNotFound
	}
	if !review.MergeAllowed {
		return nil, ErrDuplicateTVGIDManaged
	}
	keepFound := false
	for _, channel := range review.Channels {
		keepFound = keepFound || channel.ID == keepChannelID
	}
	if !keepFound {
		return nil, ErrDuplicateMergeTarget
	}

	result := &models.DuplicateTVGIDMergeResult{
		KeptChannelID: keepChannelID, MergedChannels: int64(len(review.Channels) - 1), AffectedLineups: int64(len(review.AffectedLineups)),
	}
	err = q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		affectedGroups := map[int64]bool{}
		for _, duplicate := range review.Channels {
			if duplicate.ID == keepChannelID {
				continue
			}
			sources := []duplicateMergeSource{}
			if err := tx.SelectContext(ctx, &sources, `SELECT playlist_channel_id FROM templatechannelitem WHERE channel_id = ? ORDER BY orderr, id`, duplicate.ID); err != nil {
				return err
			}
			for _, source := range sources {
				insert, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO templatechannelitem (
					channel_id, playlist_channel_id, orderr, match_method, match_score, runner_up_score, matcher_version, manual_locked
				) VALUES (?, ?, 1, ?, 1, NULL, ?, true)`, keepChannelID, source.PlaylistChannelID, channelmatch.MethodManual, channelmatch.CurrentVersion)
				if err != nil {
					return err
				}
				if moved, err := insert.RowsAffected(); err == nil {
					result.MovedSources += moved
				}
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO channelmatchrejection (channel_id, playlist_id, tvg_id_norm, name_norm, created_at)
				SELECT ?, playlist_id, tvg_id_norm, name_norm, created_at FROM channelmatchrejection WHERE channel_id = ?`, keepChannelID, duplicate.ID); err != nil {
				return err
			}
			memberships := []struct {
				GroupID int64 `db:"group_id"`
				Order   int64 `db:"orderr"`
			}{}
			if err := tx.SelectContext(ctx, &memberships, `SELECT group_id, orderr FROM template_group_channel WHERE channel_id = ?`, duplicate.ID); err != nil {
				return err
			}
			for _, membership := range memberships {
				affectedGroups[membership.GroupID] = true
				var keepExists bool
				if err := tx.GetContext(ctx, &keepExists, `SELECT EXISTS(SELECT 1 FROM template_group_channel WHERE group_id = ? AND channel_id = ?)`, membership.GroupID, keepChannelID); err != nil {
					return err
				}
				if !keepExists {
					if _, err := tx.ExecContext(ctx, `INSERT INTO template_group_channel (group_id, channel_id, orderr) VALUES (?, ?, ?)`, membership.GroupID, keepChannelID, membership.Order); err != nil {
						return err
					}
					if _, err := tx.ExecContext(ctx, `UPDATE template_group_channel SET orderr = ? WHERE group_id = ? AND channel_id = ?`, membership.Order, membership.GroupID, keepChannelID); err != nil {
						return err
					}
				}
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM templatechannel WHERE id = ?`, duplicate.ID); err != nil {
				return err
			}
		}
		variantIDs := []int64{}
		if err := tx.SelectContext(ctx, &variantIDs, `SELECT id FROM templatechannelitem WHERE channel_id = ? ORDER BY orderr, id`, keepChannelID); err != nil {
			return err
		}
		for index, id := range variantIDs {
			if _, err := tx.ExecContext(ctx, `UPDATE templatechannelitem SET orderr = ? WHERE id = ?`, index+1, id); err != nil {
				return err
			}
		}
		for groupID := range affectedGroups {
			if err := reindexWorkspaceGroup(ctx, tx, groupID); err != nil {
				return err
			}
		}
		for _, affected := range review.AffectedLineups {
			if _, err := tx.ExecContext(ctx, `DELETE FROM lineup_tvgid_review_ack WHERE lineup_id = ? AND tvg_id_norm = ?`, affected.ID, normalized); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDuplicateTVGIDNotFound
		}
		return nil, fmt.Errorf("merge duplicate TVG ID channels: %w", err)
	}
	return result, nil
}

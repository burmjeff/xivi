package queries

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/logoassets"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

var (
	ErrLineupNotFound      = errors.New("lineup not found")
	ErrStudioGroupNotFound = errors.New("studio group not found")
	ErrSourceGroupNotFound = errors.New("source group not found")
	ErrSourceGroupDisabled = errors.New("source group is disabled")
	ErrSourceGroupEmpty    = errors.New("source group is empty")
	ErrStudioGroupManaged  = errors.New("studio group is source managed")
	ErrStudioGroupName     = errors.New("studio group name is required")
)

type sourceGroupRecord struct {
	ID         int64  `db:"id"`
	Name       string `db:"name"`
	PlaylistID int64  `db:"playlist_id"`
	Enabled    bool   `db:"enabled"`
}

type syncSourceChannel struct {
	ID    int64   `db:"id"`
	Name  string  `db:"name"`
	TVGID *string `db:"tvg_id"`
}

type sourceGroupMember struct {
	TemplateChannelID int64         `db:"template_channel_id"`
	SourceChannelID   sql.NullInt64 `db:"source_channel_id"`
	SourceIdentity    string        `db:"source_identity"`
	LastSourceName    string        `db:"last_source_name"`
}

type existingWorkspaceChannel struct {
	ID    int64   `db:"id"`
	Name  string  `db:"name"`
	TVGID *string `db:"tvgid"`
}

func (q *ExperienceQueries) GetSourceGroups(ctx context.Context, playlistID *int64, search string, limit, offset int) ([]models.SourceGroup, int64, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if playlistID != nil {
		where = append(where, "p.id = ?")
		args = append(args, *playlistID)
	}
	if search != "" {
		where = append(where, "(LOWER(pg.name) LIKE ? OR LOWER(p.name) LIKE ?)")
		term := "%" + strings.ToLower(search) + "%"
		args = append(args, term, term)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := q.GetContext(ctx, &total, `SELECT COUNT(*) FROM playlistgroup pg JOIN playlist p ON p.id = pg.playlist_id WHERE `+whereSQL, args...); err != nil {
		return nil, 0, err
	}
	items := []models.SourceGroup{}
	query := `SELECT pg.id, pg.name, p.id AS playlist_id, p.name AS playlist_name, pg.enabled,
		COUNT(DISTINCT pc.id) AS channel_count,
		COUNT(DISTINCT l.group_id) AS linked_group_count
		FROM playlistgroup pg
		JOIN playlist p ON p.id = pg.playlist_id
		LEFT JOIN playlistchannel pc ON pc.group_id = pg.id
		LEFT JOIN lineup_group_source_link l ON l.source_group_id = pg.id
		WHERE ` + whereSQL + `
		GROUP BY pg.id, pg.name, p.id, p.name, pg.enabled
		ORDER BY LOWER(p.name), LOWER(pg.name), pg.id LIMIT ? OFFSET ?`
	if err := q.SelectContext(ctx, &items, query, append(args, limit, offset)...); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (q *ExperienceQueries) GetSourceGroupLink(ctx context.Context, groupID int64) (*models.SourceGroupLink, error) {
	link := &models.SourceGroupLink{}
	err := q.GetContext(ctx, link, `SELECT l.group_id, l.playlist_id, p.name AS playlist_name,
		l.source_group_id, l.source_group_name, l.follow_group_name, l.follow_channel_names,
		l.status, l.last_synced_at, l.last_error, l.added_count, l.updated_count, l.removed_count
		FROM lineup_group_source_link l
		JOIN playlist p ON p.id = l.playlist_id
		WHERE l.group_id = ?`, groupID)
	if err != nil {
		return nil, err
	}
	return link, nil
}

func (q *ExperienceQueries) IsSourceLinkedGroup(ctx context.Context, groupID int64) (bool, error) {
	var linked bool
	err := q.GetContext(ctx, &linked, `SELECT EXISTS(SELECT 1 FROM lineup_group_source_link WHERE group_id = ?)`, groupID)
	return linked, err
}

// CreateStudioGroup creates the canonical group, attaches it to the lineup,
// and optionally establishes its source subscription in one transaction. No
// empty group can escape when any part of setup fails.
func (q *ExperienceQueries) CreateStudioGroup(ctx context.Context, lineupID int64, request models.StudioGroupCreateRequest) (int64, error) {
	if strings.TrimSpace(request.Name) == "" {
		return 0, ErrStudioGroupName
	}
	var groupID int64
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var lineupExists bool
		if err := tx.GetContext(ctx, &lineupExists, `SELECT EXISTS(SELECT 1 FROM template WHERE id = ?)`, lineupID); err != nil {
			return err
		}
		if !lineupExists {
			return ErrLineupNotFound
		}
		insert, err := tx.ExecContext(ctx, `INSERT INTO templategroup (name, dynamic, dynamicgroup) VALUES (?, false, NULL)`, strings.TrimSpace(request.Name))
		if err != nil {
			return err
		}
		groupID, err = insert.LastInsertId()
		if err != nil {
			return err
		}
		var position int64
		if err := tx.GetContext(ctx, &position, `SELECT COALESCE(MAX(orderr), 0) + 1 FROM template_group_item WHERE template_id = ?`, lineupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO template_group_item (template_id, group_id, orderr) VALUES (?, ?, ?)`, lineupID, groupID, position); err != nil {
			return err
		}
		if request.SourceLink != nil {
			if request.SourceLink.SourceGroupID < 1 {
				return ErrSourceGroupNotFound
			}
			return setSourceGroupLinkTx(ctx, tx, groupID, *request.SourceLink)
		}
		return nil
	})
	return groupID, err
}

func (q *ExperienceQueries) UpdateStudioGroup(ctx context.Context, groupID int64, request models.StudioGroupUpdateRequest) error {
	if strings.TrimSpace(request.Name) == "" {
		return ErrStudioGroupName
	}
	result, err := q.ExecContext(ctx, `UPDATE templategroup SET name = ? WHERE id = ?`, strings.TrimSpace(request.Name), groupID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return ErrStudioGroupNotFound
	}
	return nil
}

func (q *ExperienceQueries) DeleteStudioGroup(ctx context.Context, groupID int64) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		channelIDs := []int64{}
		if err := tx.SelectContext(ctx, &channelIDs, `SELECT channel_id FROM template_group_channel WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM template_group_channel WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM templategroup WHERE id = ?`, groupID)
		if err != nil {
			return err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if deleted == 0 {
			return ErrStudioGroupNotFound
		}
		for _, channelID := range channelIDs {
			if _, err := tx.ExecContext(ctx, `DELETE FROM templatechannel WHERE id = ? AND NOT EXISTS (
				SELECT 1 FROM template_group_channel WHERE channel_id = ?
			)`, channelID, channelID); err != nil {
				return err
			}
		}
		return nil
	})
}

// CopySourceGroupToLineup creates a regular, manually managed lineup group
// from the source group's current snapshot. It intentionally creates no
// durable source link.
func (q *ExperienceQueries) CopySourceGroupToLineup(ctx context.Context, lineupID, sourceGroupID int64) (*models.SourceGroupImportResult, error) {
	result := &models.SourceGroupImportResult{ChannelIDs: []int64{}}
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var lineupExists bool
		if err := tx.GetContext(ctx, &lineupExists, `SELECT EXISTS(SELECT 1 FROM template WHERE id = ?)`, lineupID); err != nil {
			return err
		}
		if !lineupExists {
			return ErrLineupNotFound
		}
		source, channels, err := sourceGroupImportSnapshot(ctx, tx, sourceGroupID)
		if err != nil {
			return err
		}
		result.GroupName, err = availableStudioGroupName(ctx, tx, source.Name)
		if err != nil {
			return err
		}
		insert, err := tx.ExecContext(ctx, `INSERT INTO templategroup (name, dynamic, dynamicgroup) VALUES (?, false, NULL)`, result.GroupName)
		if err != nil {
			return err
		}
		result.GroupID, err = insert.LastInsertId()
		if err != nil {
			return err
		}
		var groupPosition int64
		if err := tx.GetContext(ctx, &groupPosition, `SELECT COALESCE(MAX(orderr), 0) + 1 FROM template_group_item WHERE template_id = ?`, lineupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO template_group_item (template_id, group_id, orderr) VALUES (?, ?, ?)`, lineupID, result.GroupID, groupPosition); err != nil {
			return err
		}
		return importSourceGroupChannels(ctx, tx, result, channels, nil)
	})
	return result, err
}

// AddSourceGroupToStudioGroup appends the source group's current snapshot to
// an existing manual group. Variants already represented in that group are
// skipped so repeating the action is idempotent.
func (q *ExperienceQueries) AddSourceGroupToStudioGroup(ctx context.Context, groupID, sourceGroupID int64) (*models.SourceGroupImportResult, error) {
	result := &models.SourceGroupImportResult{GroupID: groupID, ChannelIDs: []int64{}}
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		if err := tx.GetContext(ctx, &result.GroupName, `SELECT name FROM templategroup WHERE id = ?`, groupID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrStudioGroupNotFound
			}
			return err
		}
		var linked bool
		if err := tx.GetContext(ctx, &linked, `SELECT EXISTS(SELECT 1 FROM lineup_group_source_link WHERE group_id = ?)`, groupID); err != nil {
			return err
		}
		if linked {
			return ErrStudioGroupManaged
		}
		_, channels, err := sourceGroupImportSnapshot(ctx, tx, sourceGroupID)
		if err != nil {
			return err
		}
		existing := []int64{}
		if err := tx.SelectContext(ctx, &existing, `SELECT DISTINCT tci.playlist_channel_id
			FROM templatechannelitem tci
			JOIN template_group_channel tgc ON tgc.channel_id = tci.channel_id
			JOIN playlistchannel pc ON pc.id = tci.playlist_channel_id
			WHERE tgc.group_id = ? AND pc.group_id = ?`, groupID, sourceGroupID); err != nil {
			return err
		}
		skip := make(map[int64]bool, len(existing))
		for _, sourceChannelID := range existing {
			skip[sourceChannelID] = true
		}
		return importSourceGroupChannels(ctx, tx, result, channels, skip)
	})
	return result, err
}

func sourceGroupImportSnapshot(ctx context.Context, tx *sqlx.Tx, sourceGroupID int64) (*sourceGroupRecord, []syncSourceChannel, error) {
	source := &sourceGroupRecord{}
	if err := tx.GetContext(ctx, source, `SELECT id, name, playlist_id, enabled FROM playlistgroup WHERE id = ?`, sourceGroupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrSourceGroupNotFound
		}
		return nil, nil, err
	}
	if !source.Enabled {
		return nil, nil, ErrSourceGroupDisabled
	}
	channels := []syncSourceChannel{}
	if err := tx.SelectContext(ctx, &channels, `SELECT id, COALESCE(NULLIF(title, ''), NULLIF(tvg_name, ''), 'Untitled channel') AS name, tvg_id
		FROM playlistchannel WHERE group_id = ? ORDER BY LOWER(name), id`, sourceGroupID); err != nil {
		return nil, nil, err
	}
	if len(channels) == 0 {
		return nil, nil, ErrSourceGroupEmpty
	}
	return source, channels, nil
}

func importSourceGroupChannels(ctx context.Context, tx *sqlx.Tx, result *models.SourceGroupImportResult, channels []syncSourceChannel, skip map[int64]bool) error {
	var position int64
	if err := tx.GetContext(ctx, &position, `SELECT COALESCE(MAX(orderr), 0) FROM template_group_channel WHERE group_id = ?`, result.GroupID); err != nil {
		return err
	}
	for _, source := range channels {
		if skip[source.ID] {
			result.SkippedCount++
			continue
		}
		insert, err := tx.ExecContext(ctx, `INSERT INTO templatechannel (name, tvgid, logoid, uuid) VALUES (?, ?, 0, ?)`, source.Name, source.TVGID, uuid.NewString())
		if err != nil {
			return err
		}
		channelID, err := insert.LastInsertId()
		if err != nil {
			return err
		}
		position++
		if _, err := tx.ExecContext(ctx, `INSERT INTO template_group_channel (group_id, channel_id, orderr) VALUES (?, ?, ?)`, result.GroupID, channelID, position); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO templatechannelitem (channel_id, playlist_channel_id, orderr, match_method, match_score, runner_up_score, matcher_version, manual_locked)
			VALUES (?, ?, 1, 'manual', 1, NULL, 2, true)`, channelID, source.ID); err != nil {
			return err
		}
		result.ChannelIDs = append(result.ChannelIDs, channelID)
		result.AddedCount++
	}
	return nil
}

func availableStudioGroupName(ctx context.Context, tx *sqlx.Tx, sourceName string) (string, error) {
	base := strings.TrimSpace(sourceName)
	if base == "" {
		base = "Imported group"
	}
	for attempt := 0; ; attempt++ {
		suffix := ""
		if attempt == 1 {
			suffix = " copy"
		} else if attempt > 1 {
			suffix = fmt.Sprintf(" copy %d", attempt)
		}
		baseRunes := []rune(base)
		maxBase := 255 - len([]rune(suffix))
		if len(baseRunes) > maxBase {
			baseRunes = baseRunes[:maxBase]
		}
		candidate := string(baseRunes) + suffix
		var exists bool
		if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM templategroup WHERE LOWER(name) = LOWER(?))`, candidate); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
}

func (q *ExperienceQueries) SetSourceGroupLink(ctx context.Context, groupID int64, request models.SourceGroupLinkRequest) error {
	if groupID < 1 || request.SourceGroupID < 1 {
		return fmt.Errorf("a lineup group and source group are required")
	}
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var groupExists bool
		if err := tx.GetContext(ctx, &groupExists, `SELECT EXISTS(SELECT 1 FROM templategroup WHERE id = ?)`, groupID); err != nil {
			return err
		}
		if !groupExists {
			return ErrStudioGroupNotFound
		}
		return setSourceGroupLinkTx(ctx, tx, groupID, request)
	})
}

func setSourceGroupLinkTx(ctx context.Context, tx *sqlx.Tx, groupID int64, request models.SourceGroupLinkRequest) error {
	source := sourceGroupRecord{}
	if err := tx.GetContext(ctx, &source, `SELECT id, name, playlist_id FROM playlistgroup WHERE id = ?`, request.SourceGroupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSourceGroupNotFound
		}
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO lineup_group_source_link (
			group_id, playlist_id, source_group_id, source_group_name,
			follow_group_name, follow_channel_names, status, last_error
		) VALUES (?, ?, ?, ?, ?, ?, 'pending', '')
		ON CONFLICT(group_id) DO UPDATE SET
			playlist_id = excluded.playlist_id,
			source_group_id = excluded.source_group_id,
			source_group_name = excluded.source_group_name,
			follow_group_name = excluded.follow_group_name,
			follow_channel_names = excluded.follow_channel_names,
			status = 'pending',
			last_error = ''`, groupID, source.PlaylistID, source.ID, source.Name, request.FollowGroupName, request.FollowChannelNames)
	return err
}

func (q *ExperienceQueries) DisconnectSourceGroup(ctx context.Context, groupID int64, retainChannels bool) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		managedIDs := []int64{}
		if err := tx.SelectContext(ctx, &managedIDs, `SELECT template_channel_id FROM lineup_group_source_member WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		var exists bool
		if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM lineup_group_source_link WHERE group_id = ?)`, groupID); err != nil {
			return err
		}
		if !exists {
			return sql.ErrNoRows
		}
		for _, channelID := range managedIDs {
			if _, err := tx.ExecContext(ctx, `UPDATE templatechannelitem SET match_method = 'manual', manual_locked = true WHERE channel_id = ? AND match_method = 'source_group_sync'`, channelID); err != nil {
				return err
			}
			if !retainChannels {
				if _, err := tx.ExecContext(ctx, `DELETE FROM template_group_channel WHERE group_id = ? AND channel_id = ?`, groupID, channelID); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `DELETE FROM templatechannel WHERE id = ? AND NOT EXISTS (SELECT 1 FROM template_group_channel WHERE channel_id = ?)`, channelID, channelID); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM lineup_group_source_link WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		return reindexWorkspaceGroup(ctx, tx, groupID)
	})
}

func (q *ExperienceQueries) SyncSourceGroup(ctx context.Context, groupID int64) (*models.SourceGroupSyncResult, error) {
	link, err := q.GetSourceGroupLink(ctx, groupID)
	if err != nil {
		return nil, err
	}
	sourceGroup, err := q.resolveLinkedSourceGroup(ctx, link)
	if err != nil {
		_ = q.markSourceGroupLinkFailure(ctx, groupID, "disconnected", err)
		return nil, err
	}
	result, err := q.syncResolvedSourceGroup(ctx, link, sourceGroup)
	if err != nil {
		_ = q.markSourceGroupLinkFailure(ctx, groupID, "error", err)
		return nil, err
	}
	logoContext, cancelLogoImport := context.WithTimeout(ctx, 45*time.Second)
	defer cancelLogoImport()
	if err := q.applyDefaultSourceLogos(logoContext, groupID); err != nil {
		// Membership is already safely reconciled at this point. Keep the valid
		// lineup snapshot and retry missing logo enrichment on the next sync.
		log.Error().Err(err).Int64("group_id", groupID).Msg("Synced group logos could not be applied")
	}
	return result, nil
}

type sourceLogoCandidate struct {
	ChannelID int64  `db:"channel_id"`
	LogoURL   string `db:"logo_url"`
}

// applyDefaultSourceLogos adopts the connected source's logo only while a
// channel still uses Xivi's default. A user-selected logo is never overwritten.
func (q *ExperienceQueries) applyDefaultSourceLogos(ctx context.Context, groupID int64) error {
	candidates := []sourceLogoCandidate{}
	if err := q.SelectContext(ctx, &candidates, `SELECT tc.id AS channel_id, TRIM(pc.tvg_logo) AS logo_url
		FROM lineup_group_source_member member
		JOIN templatechannel tc ON tc.id = member.template_channel_id
		JOIN playlistchannel pc ON pc.id = member.source_channel_id
		WHERE member.group_id = ? AND COALESCE(tc.logoid, 0) = 0
			AND NULLIF(TRIM(pc.tvg_logo), '') IS NOT NULL
		ORDER BY tc.id`, groupID); err != nil {
		return err
	}
	logoIDs := map[string]int64{}
	type sourceLogoAsset struct {
		Name      string
		URL       string
		ChannelID int64
	}
	missing := []sourceLogoAsset{}
	for _, candidate := range candidates {
		name := logoassets.SourceLogoName(candidate.LogoURL)
		if _, known := logoIDs[name]; known {
			continue
		}
		var logoID int64
		err := q.GetContext(ctx, &logoID, `SELECT id FROM logo WHERE name = ?`, name)
		if err == nil {
			logoIDs[name] = logoID
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		logoIDs[name] = 0 // Dedupe repeated URLs before downloading them.
		missing = append(missing, sourceLogoAsset{Name: name, URL: candidate.LogoURL, ChannelID: candidate.ChannelID})
	}

	type sourceLogoImport struct {
		Asset sourceLogoAsset
		Err   error
	}
	jobs := make(chan sourceLogoAsset)
	imports := make(chan sourceLogoImport, len(missing))
	workerCount := min(4, len(missing))
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for asset := range jobs {
				imports <- sourceLogoImport{Asset: asset, Err: q.storeSourceLogo(ctx, asset.URL, asset.Name)}
			}
		}()
	}
	go func() {
		for _, asset := range missing {
			jobs <- asset
		}
		close(jobs)
		workers.Wait()
		close(imports)
	}()
	for imported := range imports {
		if imported.Err != nil {
			log.Warn().Err(imported.Err).Int64("channel_id", imported.Asset.ChannelID).Str("logo_url", imported.Asset.URL).Msg("Source logo could not be imported")
			continue
		}
		if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO logo (name) VALUES (?)`, imported.Asset.Name); err != nil {
			return err
		}
		var logoID int64
		if err := q.GetContext(ctx, &logoID, `SELECT id FROM logo WHERE name = ?`, imported.Asset.Name); err != nil {
			return err
		}
		logoIDs[imported.Asset.Name] = logoID
	}
	for _, candidate := range candidates {
		logoID := logoIDs[logoassets.SourceLogoName(candidate.LogoURL)]
		if logoID == 0 {
			continue
		}
		if _, err := q.ExecContext(ctx, `UPDATE templatechannel SET logoid = ? WHERE id = ? AND COALESCE(logoid, 0) = 0`, logoID, candidate.ChannelID); err != nil {
			return err
		}
	}
	return nil
}

func (q *ExperienceQueries) SyncSourceGroupsForPlaylist(ctx context.Context, playlistID int64) ([]models.SourceGroupSyncResult, error) {
	groupIDs := []int64{}
	if err := q.SelectContext(ctx, &groupIDs, `SELECT group_id FROM lineup_group_source_link WHERE playlist_id = ? ORDER BY group_id`, playlistID); err != nil {
		return nil, err
	}
	results := make([]models.SourceGroupSyncResult, 0, len(groupIDs))
	errs := []error{}
	for _, groupID := range groupIDs {
		result, err := q.SyncSourceGroup(ctx, groupID)
		if err != nil {
			errs = append(errs, fmt.Errorf("group %d: %w", groupID, err))
			continue
		}
		results = append(results, *result)
	}
	return results, errors.Join(errs...)
}

func (q *ExperienceQueries) resolveLinkedSourceGroup(ctx context.Context, link *models.SourceGroupLink) (*sourceGroupRecord, error) {
	if link.SourceGroupID != nil {
		group := &sourceGroupRecord{}
		if err := q.GetContext(ctx, group, `SELECT id, name, playlist_id FROM playlistgroup WHERE id = ? AND playlist_id = ?`, *link.SourceGroupID, link.PlaylistID); err == nil {
			return group, nil
		}
	}
	exact := &sourceGroupRecord{}
	if err := q.GetContext(ctx, exact, `SELECT id, name, playlist_id FROM playlistgroup WHERE playlist_id = ? AND LOWER(name) = LOWER(?) ORDER BY id LIMIT 1`, link.PlaylistID, link.SourceGroupName); err == nil {
		return exact, nil
	}
	memberIdentities := []string{}
	if err := q.SelectContext(ctx, &memberIdentities, `SELECT source_identity FROM lineup_group_source_member WHERE group_id = ?`, link.GroupID); err != nil {
		return nil, err
	}
	if len(memberIdentities) == 0 {
		return nil, fmt.Errorf("source group %q is no longer available; reconnect it to continue syncing", link.SourceGroupName)
	}
	wanted := map[string]bool{}
	for _, identity := range memberIdentities {
		wanted[identity] = true
	}
	candidates := []sourceGroupRecord{}
	if err := q.SelectContext(ctx, &candidates, `SELECT id, name, playlist_id FROM playlistgroup WHERE playlist_id = ? ORDER BY id`, link.PlaylistID); err != nil {
		return nil, err
	}
	var best *sourceGroupRecord
	bestScore, secondScore := 0.0, 0.0
	bestMatches := 0
	for index := range candidates {
		channels, err := q.sourceGroupChannels(ctx, candidates[index].ID)
		if err != nil {
			return nil, err
		}
		identities := sourceChannelIdentities(channels)
		matches := 0
		for _, identity := range identities {
			if wanted[identity] {
				matches++
			}
		}
		score := float64(matches) / math.Max(float64(len(wanted)), float64(len(identities)))
		if score > bestScore {
			secondScore = bestScore
			bestScore, bestMatches, best = score, matches, &candidates[index]
		} else if score > secondScore {
			secondScore = score
		}
	}
	if best != nil && bestMatches > 0 && bestScore >= 0.5 && bestScore-secondScore >= 0.1 {
		return best, nil
	}
	return nil, fmt.Errorf("source group %q is no longer available and no unambiguous replacement was found", link.SourceGroupName)
}

func (q *ExperienceQueries) sourceGroupChannels(ctx context.Context, sourceGroupID int64) ([]syncSourceChannel, error) {
	channels := []syncSourceChannel{}
	err := q.SelectContext(ctx, &channels, `SELECT id, COALESCE(NULLIF(title, ''), NULLIF(tvg_name, ''), 'Untitled channel') AS name, tvg_id
		FROM playlistchannel WHERE group_id = ? ORDER BY LOWER(name), id`, sourceGroupID)
	return channels, err
}

func sourceChannelIdentities(channels []syncSourceChannel) []string {
	bases := make([]string, len(channels))
	counts := map[string]int{}
	for index, channel := range channels {
		base := "source:" + strconv.FormatInt(channel.ID, 10)
		if channel.TVGID != nil && strings.TrimSpace(*channel.TVGID) != "" {
			base = "tvg:" + strings.ToLower(strings.TrimSpace(*channel.TVGID))
		}
		bases[index] = base
		counts[base]++
	}
	for index, base := range bases {
		if counts[base] > 1 {
			bases[index] = base + "#" + strconv.FormatInt(channels[index].ID, 10)
		}
	}
	return bases
}

func (q *ExperienceQueries) syncResolvedSourceGroup(ctx context.Context, link *models.SourceGroupLink, sourceGroup *sourceGroupRecord) (*models.SourceGroupSyncResult, error) {
	result := &models.SourceGroupSyncResult{GroupID: link.GroupID}
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		channels := []syncSourceChannel{}
		if err := tx.SelectContext(ctx, &channels, `SELECT id, COALESCE(NULLIF(title, ''), NULLIF(tvg_name, ''), 'Untitled channel') AS name, tvg_id
			FROM playlistchannel WHERE group_id = ? ORDER BY LOWER(name), id`, sourceGroup.ID); err != nil {
			return err
		}
		if len(channels) == 0 {
			return fmt.Errorf("source group %q is empty; the previous lineup snapshot was retained", sourceGroup.Name)
		}
		identities := sourceChannelIdentities(channels)
		members := []sourceGroupMember{}
		if err := tx.SelectContext(ctx, &members, `SELECT template_channel_id, source_channel_id, source_identity, last_source_name
			FROM lineup_group_source_member WHERE group_id = ?`, link.GroupID); err != nil {
			return err
		}
		bySourceID := map[int64]*sourceGroupMember{}
		byIdentity := map[string]*sourceGroupMember{}
		for index := range members {
			member := &members[index]
			if member.SourceChannelID.Valid {
				bySourceID[member.SourceChannelID.Int64] = member
			}
			byIdentity[member.SourceIdentity] = member
		}
		existing := []existingWorkspaceChannel{}
		if err := tx.SelectContext(ctx, &existing, `SELECT tc.id, tc.name, tc.tvgid FROM templatechannel tc
			JOIN template_group_channel tgc ON tgc.channel_id = tc.id WHERE tgc.group_id = ? ORDER BY tgc.orderr, tc.id`, link.GroupID); err != nil {
			return err
		}
		existingByID := map[int64]existingWorkspaceChannel{}
		claimed := map[int64]bool{}
		for _, channel := range existing {
			existingByID[channel.ID] = channel
		}
		for _, member := range members {
			claimed[member.TemplateChannelID] = true
		}
		seenMembers := map[int64]bool{}
		for index, source := range channels {
			identity := identities[index]
			member := bySourceID[source.ID]
			if member == nil {
				member = byIdentity[identity]
			}
			channelID := int64(0)
			changed := false
			created := false
			if member != nil {
				channelID = member.TemplateChannelID
				changed = !member.SourceChannelID.Valid || member.SourceChannelID.Int64 != source.ID || member.SourceIdentity != identity || member.LastSourceName != source.Name
			} else {
				for _, candidate := range existing {
					if claimed[candidate.ID] {
						continue
					}
					matched := source.TVGID != nil && candidate.TVGID != nil && strings.EqualFold(strings.TrimSpace(*source.TVGID), strings.TrimSpace(*candidate.TVGID))
					if !matched {
						if err := tx.GetContext(ctx, &matched, `SELECT EXISTS(SELECT 1 FROM templatechannelitem WHERE channel_id = ? AND playlist_channel_id = ?)`, candidate.ID, source.ID); err != nil {
							return err
						}
					}
					if matched {
						channelID = candidate.ID
						claimed[channelID] = true
						changed = true
						break
					}
				}
				if channelID == 0 {
					insert, err := tx.ExecContext(ctx, `INSERT INTO templatechannel (name, tvgid, logoid, uuid) VALUES (?, ?, 0, ?)`, source.Name, source.TVGID, uuid.NewString())
					if err != nil {
						return err
					}
					channelID, err = insert.LastInsertId()
					if err != nil {
						return err
					}
					result.AddedCount++
					created = true
				}
			}
			current := existingByID[channelID]
			if link.FollowChannelNames && current.Name != "" && current.Name != source.Name {
				if _, err := tx.ExecContext(ctx, `UPDATE templatechannel SET name = ? WHERE id = ?`, source.Name, channelID); err != nil {
					return err
				}
				changed = true
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO template_group_channel (group_id, channel_id, orderr) VALUES (?, ?, ?)`, link.GroupID, channelID, index+1); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE template_group_channel SET orderr = ? WHERE group_id = ? AND channel_id = ?`, index+1, link.GroupID, channelID); err != nil {
				return err
			}
			if member == nil {
				if _, err := tx.ExecContext(ctx, `INSERT INTO lineup_group_source_member (group_id, template_channel_id, source_channel_id, source_identity, last_source_name) VALUES (?, ?, ?, ?, ?)`, link.GroupID, channelID, source.ID, identity, source.Name); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `UPDATE lineup_group_source_member SET source_channel_id = ?, source_identity = ?, last_source_name = ? WHERE group_id = ? AND template_channel_id = ?`, source.ID, identity, source.Name, link.GroupID, channelID); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM templatechannelitem WHERE channel_id = ? AND playlist_channel_id IN (
				SELECT pc.id FROM playlistchannel pc JOIN playlistgroup pg ON pg.id = pc.group_id WHERE pg.playlist_id = ?
			)`, channelID, link.PlaylistID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO templatechannelitem (channel_id, playlist_channel_id, orderr, match_method, match_score, runner_up_score, matcher_version, manual_locked)
				VALUES (?, ?, 1, 'source_group_sync', 1, NULL, 2, true)`, channelID, source.ID); err != nil {
				return err
			}
			if !created && changed {
				result.UpdatedCount++
			}
			seenMembers[channelID] = true
		}
		for _, member := range members {
			if seenMembers[member.TemplateChannelID] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM lineup_group_source_member WHERE group_id = ? AND template_channel_id = ?`, link.GroupID, member.TemplateChannelID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM template_group_channel WHERE group_id = ? AND channel_id = ?`, link.GroupID, member.TemplateChannelID); err != nil {
				return err
			}
			var stillUsed bool
			if err := tx.GetContext(ctx, &stillUsed, `SELECT EXISTS(SELECT 1 FROM template_group_channel WHERE channel_id = ?)`, member.TemplateChannelID); err != nil {
				return err
			}
			if stillUsed {
				if _, err := tx.ExecContext(ctx, `UPDATE templatechannelitem SET match_method = 'manual', manual_locked = true WHERE channel_id = ? AND match_method = 'source_group_sync'`, member.TemplateChannelID); err != nil {
					return err
				}
			} else if _, err := tx.ExecContext(ctx, `DELETE FROM templatechannel WHERE id = ?`, member.TemplateChannelID); err != nil {
				return err
			}
			result.RemovedCount++
		}
		if link.FollowGroupName {
			var conflict bool
			if err := tx.GetContext(ctx, &conflict, `SELECT EXISTS(SELECT 1 FROM templategroup WHERE LOWER(name) = LOWER(?) AND id != ?)`, sourceGroup.Name, link.GroupID); err != nil {
				return err
			}
			if !conflict {
				if _, err := tx.ExecContext(ctx, `UPDATE templategroup SET name = ? WHERE id = ?`, sourceGroup.Name, link.GroupID); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE lineup_group_source_link SET
			source_group_id = ?, source_group_name = ?, status = 'active', last_synced_at = datetime('now'),
			last_error = '', added_count = ?, updated_count = ?, removed_count = ? WHERE group_id = ?`,
			sourceGroup.ID, sourceGroup.Name, result.AddedCount, result.UpdatedCount, result.RemovedCount, link.GroupID); err != nil {
			return err
		}
		return nil
	})
	return result, err
}

func (q *ExperienceQueries) markSourceGroupLinkFailure(ctx context.Context, groupID int64, status string, syncErr error) error {
	message := "The synced group could not be updated."
	if syncErr != nil && strings.TrimSpace(syncErr.Error()) != "" {
		message = syncErr.Error()
	}
	_, err := q.ExecContext(ctx, `UPDATE lineup_group_source_link SET status = ?, last_error = ? WHERE group_id = ?`, status, message, groupID)
	return err
}

package queries

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/channelmatch"
	"xivi/backend/pkg/logoassets"
	"xivi/backend/pkg/security"

	"github.com/jmoiron/sqlx"
)

// ExperienceQueries powers the additive API used by the new interface.
type ExperienceQueries struct {
	BaseQueries
	storeSourceLogo func(context.Context, string, string) error
}

func NewExperienceQueries(db *sqlx.DB) *ExperienceQueries {
	return &ExperienceQueries{
		BaseQueries: NewBaseQueries(db),
		storeSourceLogo: func(ctx context.Context, rawURL, name string) error {
			return logoassets.StoreSourceLogo(ctx, rawURL, name)
		},
	}
}

func revealOptionalProviderValue(value **string) error {
	if value == nil || *value == nil {
		return nil
	}
	revealed, err := security.RevealString(**value)
	if err != nil {
		return err
	}
	*value = &revealed
	return nil
}

func (q *ExperienceQueries) GetLineupSummaries(ctx context.Context) ([]models.LineupSummary, error) {
	return q.getLineupSummaries(ctx, 0, false)
}

// GetViewerLineupSummaries enforces lineup grants in SQL so unauthorized rows
// never enter the application response pipeline.
func (q *ExperienceQueries) GetViewerLineupSummaries(ctx context.Context, userID int64) ([]models.LineupSummary, error) {
	return q.getLineupSummaries(ctx, userID, true)
}

func (q *ExperienceQueries) getLineupSummaries(ctx context.Context, userID int64, restricted bool) ([]models.LineupSummary, error) {
	rows := []models.LineupSummary{}
	query := `
		SELECT t.id, t.name,
		       COUNT(DISTINCT tgi.group_id) AS group_count,
		       COUNT(DISTINCT tgc.channel_id) AS channel_count,
		       CASE WHEN COUNT(DISTINCT tgc.channel_id) = 0 THEN 0
		            ELSE ROUND(100.0 * COUNT(DISTINCT CASE WHEN tc.tvgid IS NOT NULL AND tc.tvgid != '' THEN tc.id END)
		                 / COUNT(DISTINCT tgc.channel_id), 1) END AS epg_coverage
		FROM template t
		LEFT JOIN template_group_item tgi ON tgi.template_id = t.id
		LEFT JOIN template_group_channel tgc ON tgc.group_id = tgi.group_id
		LEFT JOIN templatechannel tc ON tc.id = tgc.channel_id`
	args := []any{}
	if restricted {
		query += ` WHERE EXISTS (SELECT 1 FROM user_lineup ul WHERE ul.user_id = ? AND ul.lineup_id = t.id)`
		args = append(args, userID)
	}
	query += `
		GROUP BY t.id, t.name
		ORDER BY LOWER(t.name), t.id`
	if err := q.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	duplicates, err := q.loadDuplicateTVGIDReviews(ctx, nil, false)
	if err != nil {
		return nil, err
	}
	counts := map[int64]int64{}
	for _, duplicate := range duplicates {
		counts[duplicate.LineupID]++
	}
	for index := range rows {
		rows[index].DuplicateTVGIDCount = counts[rows[index].ID]
	}
	return rows, nil
}

func (q *ExperienceQueries) GetLineupGroups(ctx context.Context, lineupID int64) ([]models.StudioGroupSummary, error) {
	rows := []models.StudioGroupSummary{}
	query := `
		SELECT tg.id, tg.name, tgi.orderr, COUNT(tgc.channel_id) AS channel_count
		FROM template_group_item tgi
		JOIN templategroup tg ON tg.id = tgi.group_id
		LEFT JOIN template_group_channel tgc ON tgc.group_id = tg.id
		WHERE tgi.template_id = ?
		GROUP BY tg.id, tg.name, tgi.orderr
		ORDER BY tgi.orderr, tg.id`
	if err := q.SelectContext(ctx, &rows, query, lineupID); err != nil {
		return nil, err
	}
	for index := range rows {
		link, err := q.GetSourceGroupLink(ctx, rows[index].ID)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		rows[index].SourceLink = link
	}
	return rows, nil
}

func (q *ExperienceQueries) GetWatchLineupGroups(ctx context.Context, lineupID int64) ([]models.WatchGroupSummary, error) {
	rows := []models.WatchGroupSummary{}
	query := `
		SELECT tg.id, tg.name, tgi.orderr, COUNT(tgc.channel_id) AS channel_count
		FROM template_group_item tgi
		JOIN templategroup tg ON tg.id = tgi.group_id
		JOIN template_group_channel tgc ON tgc.group_id = tg.id
		WHERE tgi.template_id = ?
		  AND EXISTS (SELECT 1 FROM templatechannelitem ti WHERE ti.channel_id = tgc.channel_id)
		GROUP BY tg.id, tg.name, tgi.orderr
		HAVING COUNT(tgc.channel_id) > 0
		ORDER BY tgi.orderr, tg.id`
	return rows, q.SelectContext(ctx, &rows, query, lineupID)
}

func (q *ExperienceQueries) GetGuideChannels(ctx context.Context, lineupID int64, groupID *int64, search string, from, to time.Time, limit, offset int) ([]models.GuideChannel, int64, error) {
	return q.GetGuideChannelsWithOptions(ctx, lineupID, groupID, search, from, to, limit, offset, false, nil)
}

func (q *ExperienceQueries) GetGuideChannelsWithOptions(ctx context.Context, lineupID int64, groupID *int64, search string, from, to time.Time, limit, offset int, metadataOnly bool, channelIDs []int64, onNowCategory ...string) ([]models.GuideChannel, int64, error) {
	where := []string{"tgi.template_id = ?", "EXISTS (SELECT 1 FROM templatechannelitem ti WHERE ti.channel_id = tc.id)"}
	args := []any{lineupID}
	pageWhere := []string{}
	pageFilterArgs := []any{}
	if len(onNowCategory) > 0 && strings.TrimSpace(onNowCategory[0]) != "" {
		category := strings.ToLower(strings.TrimSpace(onNowCategory[0]))
		if len(category) > 80 {
			return nil, 0, fmt.Errorf("category is too long")
		}
		now := epgQueryTime(time.Now().UTC())
		where = append(where, `EXISTS(SELECT 1 FROM epgprogramme p WHERE p.channel=tc.tvgid AND p.start<=? AND p.stop>? AND instr(','||replace(lower(COALESCE(p.categories,'')),', ', ',')||',',?)>0)`)
		pageWhere = append(pageWhere, `EXISTS(SELECT 1 FROM epgprogramme p WHERE p.channel=tvgid AND p.start<=? AND p.stop>? AND instr(','||replace(lower(COALESCE(p.categories,'')),', ', ',')||',',?)>0)`)
		args = append(args, now, now, ","+category+",")
		pageFilterArgs = append(pageFilterArgs, now, now, ","+category+",")
	}
	if len(channelIDs) > 100 {
		return nil, 0, fmt.Errorf("at most 100 channel IDs may be requested")
	}
	if len(channelIDs) > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(channelIDs)), ",")
		where = append(where, "tc.id IN ("+marks+")")
		pageWhere = append(pageWhere, "id IN ("+marks+")")
		for _, id := range channelIDs {
			args = append(args, id)
			pageFilterArgs = append(pageFilterArgs, id)
		}
	}
	if groupID != nil {
		where = append(where, "tg.id = ?")
		args = append(args, *groupID)
		pageWhere = append(pageWhere, "group_id = ?")
		pageFilterArgs = append(pageFilterArgs, *groupID)
	}
	if search != "" {
		where = append(where, "(LOWER(tc.name) LIKE ? OR LOWER(COALESCE(tc.tvgid, '')) LIKE ?)")
		term := "%" + strings.ToLower(search) + "%"
		args = append(args, term, term)
		pageWhere = append(pageWhere, "(LOWER(name) LIKE ? OR LOWER(COALESCE(tvgid, '')) LIKE ?)")
		pageFilterArgs = append(pageFilterArgs, term, term)
	}
	whereSQL := strings.Join(where, " AND ")
	pageWhereSQL := ""
	if len(pageWhere) > 0 {
		pageWhereSQL = " WHERE " + strings.Join(pageWhere, " AND ")
	}

	var total int64
	countQuery := `SELECT COUNT(*) FROM templatechannel tc
		JOIN template_group_channel tgc ON tgc.channel_id = tc.id
		JOIN templategroup tg ON tg.id = tgc.group_id
		JOIN template_group_item tgi ON tgi.group_id = tg.id
		WHERE ` + whereSQL
	if err := q.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	pageArgs := append([]any{lineupID}, pageFilterArgs...)
	pageArgs = append(pageArgs, limit, offset)
	query := `
		WITH ordered AS (
			SELECT tc.id, tc.name, tc.tvgid, tc.uuid, COALESCE(l.name, '') AS logo,
			       tg.id AS group_id, tg.name AS group_name,
			       ROW_NUMBER() OVER (ORDER BY tgi.orderr, tgc.orderr, tc.id) AS number
			FROM templatechannel tc
			JOIN template_group_channel tgc ON tgc.channel_id = tc.id
			JOIN templategroup tg ON tg.id = tgc.group_id
			JOIN template_group_item tgi ON tgi.group_id = tg.id
			LEFT JOIN logo l ON l.id = tc.logoid
			WHERE tgi.template_id = ?
			  AND EXISTS (SELECT 1 FROM templatechannelitem ti WHERE ti.channel_id = tc.id)
		)
		SELECT * FROM ordered` + pageWhereSQL + ` ORDER BY number LIMIT ? OFFSET ?`
	channels := []models.GuideChannel{}
	if err := q.SelectContext(ctx, &channels, query, pageArgs...); err != nil {
		return nil, 0, err
	}

	if metadataOnly {
		to = from
	}
	if err := q.enrichGuideChannels(ctx, channels, from, to); err != nil {
		return nil, 0, err
	}
	return channels, total, nil
}

func (q *ExperienceQueries) enrichGuideChannels(ctx context.Context, channels []models.GuideChannel, from, to time.Time) error {
	tvgIDs := make([]string, 0, len(channels))
	for i := range channels {
		channels[i].StreamURL = "/stream/hls/" + channels[i].UUID
		channels[i].Programmes = []models.Programme{}
		if channels[i].Logo == "xivi_channel" {
			channels[i].Logo = ""
		} else if channels[i].Logo != "" {
			channels[i].Logo = "/images/" + channels[i].Logo + ".png"
		}
		if channels[i].TVGID != nil && *channels[i].TVGID != "" {
			tvgIDs = append(tvgIDs, *channels[i].TVGID)
		}
	}
	if len(tvgIDs) == 0 || !to.After(from) {
		return nil
	}

	programmeQuery, programmeArgs, err := sqlx.In(`
		SELECT id, channel, COALESCE("title.value", '') AS title,
		       COALESCE(subtitle, '') AS subtitle, COALESCE(desc, '') AS description,
		       COALESCE(categories, '') AS categories, start, stop AS end
		FROM epgprogramme
		WHERE channel IN (?) AND start < ? AND stop > ?
		ORDER BY channel, start`, tvgIDs, epgQueryTime(to), epgQueryTime(from))
	if err != nil {
		return err
	}
	programmes := []models.Programme{}
	if err := q.SelectContext(ctx, &programmes, q.Rebind(programmeQuery), programmeArgs...); err != nil {
		return err
	}
	byTVG := make(map[string][]models.Programme, len(tvgIDs))
	now := time.Now()
	for i := range programmes {
		programmes[i].Start = programmes[i].Start.UTC()
		programmes[i].End = programmes[i].End.UTC()
		if programmes[i].Categories != "" {
			programmes[i].Category = strings.Split(programmes[i].Categories, ",")
		} else {
			programmes[i].Category = []string{}
		}
		byTVG[programmes[i].ChannelID] = append(byTVG[programmes[i].ChannelID], programmes[i])
	}
	for i := range channels {
		if channels[i].TVGID == nil {
			continue
		}
		if scheduled, ok := byTVG[*channels[i].TVGID]; ok {
			channels[i].Programmes = scheduled
		}
		for p := range channels[i].Programmes {
			programme := &channels[i].Programmes[p]
			if !programme.Start.After(now) && programme.End.After(now) {
				channels[i].Current = programme
			} else if programme.Start.After(now) && channels[i].Next == nil {
				channels[i].Next = programme
			}
		}
	}
	return nil
}

func (q *ExperienceQueries) GetGuideChannel(ctx context.Context, channelID int64, from, to time.Time) (*models.GuideChannel, error) {
	var lineupID int64
	err := q.GetContext(ctx, &lineupID, `SELECT tgi.template_id FROM template_group_channel tgc JOIN template_group_item tgi ON tgi.group_id = tgc.group_id WHERE tgc.channel_id = ? ORDER BY tgi.template_id LIMIT 1`, channelID)
	if err != nil {
		return nil, err
	}
	return q.GetGuideChannelForLineup(ctx, lineupID, channelID, from, to)
}

func (q *ExperienceQueries) GetGuideChannelForLineup(ctx context.Context, lineupID, channelID int64, from, to time.Time) (*models.GuideChannel, error) {
	channels := []models.GuideChannel{}
	err := q.SelectContext(ctx, &channels, `
		WITH ordered AS (
			SELECT tc.id, tc.name, tc.tvgid, tc.uuid, COALESCE(l.name, '') AS logo,
			       tg.id AS group_id, tg.name AS group_name,
			       ROW_NUMBER() OVER (ORDER BY tgi.orderr, tgc.orderr, tc.id) AS number
			FROM templatechannel tc
			JOIN template_group_channel tgc ON tgc.channel_id = tc.id
			JOIN templategroup tg ON tg.id = tgc.group_id
			JOIN template_group_item tgi ON tgi.group_id = tg.id
			LEFT JOIN logo l ON l.id = tc.logoid
			WHERE tgi.template_id = ?
			  AND EXISTS (SELECT 1 FROM templatechannelitem ti WHERE ti.channel_id = tc.id)
		)
		SELECT * FROM ordered WHERE id = ?`, lineupID, channelID)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, sql.ErrNoRows
	}
	if err := q.enrichGuideChannels(ctx, channels, from, to); err != nil {
		return nil, err
	}
	return &channels[0], nil
}

func (q *ExperienceQueries) GetWatchChannelNeighbors(ctx context.Context, lineupID, channelID int64) (*models.WatchChannelNeighbors, error) {
	current := struct {
		Number int64 `db:"number"`
	}{}
	if err := q.GetContext(ctx, &current, `
		WITH ordered AS (
			SELECT tc.id, ROW_NUMBER() OVER (ORDER BY tgi.orderr, tgc.orderr, tc.id) AS number
			FROM templatechannel tc
			JOIN template_group_channel tgc ON tgc.channel_id = tc.id
			JOIN template_group_item tgi ON tgi.group_id = tgc.group_id
			WHERE tgi.template_id = ?
			  AND EXISTS (SELECT 1 FROM templatechannelitem ti WHERE ti.channel_id = tc.id)
		)
		SELECT number FROM ordered WHERE id = ?`, lineupID, channelID); err != nil {
		return nil, err
	}

	channels := []models.GuideChannel{}
	if err := q.SelectContext(ctx, &channels, `
		WITH ordered AS (
			SELECT tc.id, tc.name, tc.tvgid, tc.uuid, COALESCE(l.name, '') AS logo,
			       tg.id AS group_id, tg.name AS group_name,
			       ROW_NUMBER() OVER (ORDER BY tgi.orderr, tgc.orderr, tc.id) AS number
			FROM templatechannel tc
			JOIN template_group_channel tgc ON tgc.channel_id = tc.id
			JOIN templategroup tg ON tg.id = tgc.group_id
			JOIN template_group_item tgi ON tgi.group_id = tg.id
			LEFT JOIN logo l ON l.id = tc.logoid
			WHERE tgi.template_id = ?
			  AND EXISTS (SELECT 1 FROM templatechannelitem ti WHERE ti.channel_id = tc.id)
		)
		SELECT * FROM ordered WHERE number IN (?, ?) ORDER BY number`, lineupID, current.Number-1, current.Number+1); err != nil {
		return nil, err
	}
	if err := q.enrichGuideChannels(ctx, channels, time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(4*time.Hour)); err != nil {
		return nil, err
	}
	result := &models.WatchChannelNeighbors{}
	for i := range channels {
		if channels[i].Number < current.Number {
			result.Previous = &channels[i]
		} else {
			result.Next = &channels[i]
		}
	}
	return result, nil
}

func (q *ExperienceQueries) GetWorkspaceChannels(ctx context.Context, groupID int64, lineupID *int64, search, matchHealth string, limit, offset int) ([]models.WorkspaceChannel, int64, error) {
	where, args := "tgc.group_id = ?", []any{groupID}
	switch matchHealth {
	case "":
	case "unmatched":
		where += " AND NOT EXISTS (SELECT 1 FROM templatechannelitem health_item WHERE health_item.channel_id = tc.id)"
	case "low-confidence":
		where += ` AND NOT EXISTS (SELECT 1 FROM templatechannelitem locked_item WHERE locked_item.channel_id = tc.id AND locked_item.manual_locked = 1)
			AND EXISTS (SELECT 1 FROM templatechannelitem review_item WHERE review_item.channel_id = tc.id AND review_item.manual_locked = 0 AND review_item.match_score IS NOT NULL AND review_item.match_score < 0.82)`
	case "duplicate-tvg-id":
		if lineupID == nil {
			return nil, 0, fmt.Errorf("lineup_id is required for duplicate TVG ID review")
		}
		duplicates, err := q.loadDuplicateTVGIDReviews(ctx, lineupID, false)
		if err != nil {
			return nil, 0, err
		}
		channelIDs := []int64{}
		seen := map[int64]bool{}
		for _, duplicate := range duplicates {
			for _, channel := range duplicate.Channels {
				if !seen[channel.ID] {
					seen[channel.ID] = true
					channelIDs = append(channelIDs, channel.ID)
				}
			}
		}
		if len(channelIDs) == 0 {
			return []models.WorkspaceChannel{}, 0, nil
		}
		where += " AND tc.id IN (?)"
		args = append(args, channelIDs)
	default:
		return nil, 0, fmt.Errorf("unsupported match health filter %q", matchHealth)
	}
	if search != "" {
		where += " AND (LOWER(tc.name) LIKE ? OR LOWER(COALESCE(tc.tvgid, '')) LIKE ?)"
		term := "%" + strings.ToLower(search) + "%"
		args = append(args, term, term)
	}
	var total int64
	countQuery, countArgs, err := sqlx.In(`SELECT COUNT(*) FROM template_group_channel tgc JOIN templatechannel tc ON tc.id = tgc.channel_id WHERE `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	if err := q.GetContext(ctx, &total, q.Rebind(countQuery), countArgs...); err != nil {
		return nil, 0, err
	}
	query := `SELECT tc.id, tc.name, tc.tvgid, tc.uuid, tc.logoid AS logo_id, COALESCE(l.name, '') AS logo, tgc.orderr,
		COUNT(tci.id) AS source_count, COALESCE(MAX(tci.match_method), '') AS match_method,
		MAX(tci.match_score) AS match_score, MAX(tci.runner_up_score) AS runner_up_score,
		COALESCE(MAX(tci.manual_locked), 0) AS manual_locked
		FROM template_group_channel tgc
		JOIN templatechannel tc ON tc.id = tgc.channel_id
		LEFT JOIN templatechannelitem tci ON tci.channel_id = tc.id
		LEFT JOIN logo l ON l.id = tc.logoid
		WHERE ` + where + ` GROUP BY tc.id, tc.name, tc.tvgid, tc.uuid, tc.logoid, l.name, tgc.orderr
		ORDER BY tgc.orderr, tc.id LIMIT ? OFFSET ?`
	query, queryArgs, err := sqlx.In(query, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	rows := []models.WorkspaceChannel{}
	if err := q.SelectContext(ctx, &rows, q.Rebind(query), queryArgs...); err != nil {
		return nil, 0, err
	}
	for i := range rows {
		if rows[i].Logo == "xivi_channel" {
			rows[i].Logo = ""
		} else if rows[i].Logo != "" {
			rows[i].Logo = "/images/" + rows[i].Logo + ".png"
		}
	}
	return rows, total, nil
}

func (q *ExperienceQueries) UpdateWorkspaceChannel(ctx context.Context, id int64, update models.WorkspaceChannelUpdate) error {
	logoID := int64(0)
	if update.LogoID != nil {
		logoID = *update.LogoID
	} else if err := q.GetContext(ctx, &logoID, `SELECT logoid FROM templatechannel WHERE id = ?`, id); err != nil {
		return err
	}
	result, err := q.ExecContext(ctx, `UPDATE templatechannel SET name = ?, tvgid = ?, logoid = ? WHERE id = ?`, strings.TrimSpace(update.Name), update.TVGID, logoID, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (q *ExperienceQueries) SetSourceChannelsEnabled(ctx context.Context, ids []int64, enabled bool) error {
	if len(ids) == 0 {
		return fmt.Errorf("at least one channel_id is required")
	}
	query, args, err := sqlx.In(`UPDATE playlistchannel SET enabled = ?, updated_at = datetime('now','localtime') WHERE id IN (?)`, enabled, ids)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, q.Rebind(query), args...)
	return err
}

func (q *ExperienceQueries) GetSourceChannels(ctx context.Context, playlistID, groupID, lineupID *int64, unusedOnly, enabledOnly bool, search string, limit, offset int) ([]models.SourceChannel, int64, error) {
	where, args := []string{"1 = 1"}, []any{}
	if playlistID != nil {
		where = append(where, "p.id = ?")
		args = append(args, *playlistID)
	}
	if groupID != nil {
		where = append(where, "pg.id = ?")
		args = append(args, *groupID)
	}
	if enabledOnly {
		where = append(where, "pc.enabled = true", "pg.enabled = true")
	}
	if search != "" {
		where = append(where, "(LOWER(COALESCE(pc.tvg_name, pc.title)) LIKE ? OR LOWER(COALESCE(pc.tvg_id, '')) LIKE ?)")
		term := "%" + strings.ToLower(search) + "%"
		args = append(args, term, term)
	}
	if unusedOnly && lineupID != nil {
		where = append(where, `pc.id NOT IN (
			SELECT tci.playlist_channel_id
			FROM template_group_item tgi
			JOIN template_group_channel tgc ON tgc.group_id = tgi.group_id
			JOIN templatechannelitem tci ON tci.channel_id = tgc.channel_id
			WHERE tgi.template_id = ?
		)`)
		args = append(args, *lineupID)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := q.GetContext(ctx, &total, `SELECT COUNT(*) FROM playlistchannel pc JOIN playlistgroup pg ON pg.id = pc.group_id JOIN playlist p ON p.id = pg.playlist_id WHERE `+whereSQL, args...); err != nil {
		return nil, 0, err
	}
	query := `SELECT pc.id, COALESCE(NULLIF(pc.tvg_name, ''), pc.title) AS name, pc.tvg_id,
		pc.tvg_logo AS logo_url, pc.enabled, pg.id AS group_id, pg.name AS group_name,
		p.id AS playlist_id, p.name AS playlist_name
		FROM playlistchannel pc JOIN playlistgroup pg ON pg.id = pc.group_id JOIN playlist p ON p.id = pg.playlist_id
		WHERE ` + whereSQL + ` ORDER BY LOWER(p.name), LOWER(pg.name), LOWER(COALESCE(NULLIF(pc.tvg_name, ''), pc.title)), pc.id LIMIT ? OFFSET ?`
	rows := []models.SourceChannel{}
	if err := q.SelectContext(ctx, &rows, query, append(args, limit, offset)...); err != nil {
		return nil, 0, err
	}
	for index := range rows {
		if err := revealOptionalProviderValue(&rows[index].LogoURL); err != nil {
			return nil, 0, err
		}
	}
	return rows, total, nil
}

func (q *ExperienceQueries) GetSourceChannelLogoSource(ctx context.Context, sourceChannelID int64) (string, error) {
	var source struct {
		URL string `db:"url"`
	}
	err := q.GetContext(ctx, &source, `SELECT TRIM(pc.tvg_logo) AS url
		FROM playlistchannel pc
		JOIN playlistgroup pg ON pg.id = pc.group_id
		WHERE pc.id = ? AND NULLIF(TRIM(pc.tvg_logo), '') IS NOT NULL`, sourceChannelID)
	if err != nil {
		return "", err
	}
	source.URL, err = security.RevealString(source.URL)
	return source.URL, err
}

func (q *ExperienceQueries) GetMatchReview(ctx context.Context, channelID *int64, search string, limit, offset int) ([]models.MatchReview, int64, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if channelID != nil {
		where = append(where, "tc.id = ?")
		args = append(args, *channelID)
	} else {
		where = append(where, "tci.manual_locked = 0 AND (tci.match_score IS NULL OR tci.match_score < 0.82)")
	}
	if search != "" {
		where = append(where, "(LOWER(tc.name) LIKE ? OR LOWER(COALESCE(pc.tvg_name, pc.title)) LIKE ? OR LOWER(p.name) LIKE ?)")
		term := "%" + strings.ToLower(search) + "%"
		args = append(args, term, term, term)
	}
	whereSQL := strings.Join(where, " AND ")
	joinSQL := ` FROM templatechannelitem tci
		JOIN templatechannel tc ON tc.id = tci.channel_id
		JOIN playlistchannel pc ON pc.id = tci.playlist_channel_id
		JOIN playlistgroup pg ON pg.id = pc.group_id
		JOIN playlist p ON p.id = pg.playlist_id `
	var total int64
	if err := q.GetContext(ctx, &total, `SELECT COUNT(*)`+joinSQL+`WHERE `+whereSQL, args...); err != nil {
		return nil, 0, err
	}
	rows := []models.MatchReview{}
	orderSQL := `tci.manual_locked DESC, COALESCE(tci.match_score, -1) DESC, LOWER(source_name), pc.id`
	if channelID != nil {
		orderSQL = `tci.orderr, tci.id`
	}
	query := `SELECT tc.id AS channel_id, tc.name AS channel_name, pc.id AS source_channel_id,
		COALESCE(NULLIF(pc.tvg_name, ''), pc.title) AS source_name, pc.tvg_logo AS source_logo_url,
		pg.id AS group_id, pg.name AS group_name, p.id AS playlist_id, p.name AS playlist_name,
		tci.orderr, tci.match_method AS method, tci.match_score AS score, tci.runner_up_score,
		tci.manual_locked ` + joinSQL + ` WHERE ` + whereSQL + `
		ORDER BY ` + orderSQL + ` LIMIT ? OFFSET ?`
	if err := q.SelectContext(ctx, &rows, query, append(args, limit, offset)...); err != nil {
		return nil, 0, err
	}
	for index := range rows {
		if err := revealOptionalProviderValue(&rows[index].SourceLogoURL); err != nil {
			return nil, 0, err
		}
	}
	return rows, total, nil
}

type sourceMatchCandidate struct {
	models.MatchSuggestion
	Title string `db:"title"`
}

// GetMatchSuggestions ranks eligible sources for a lineup channel. Only the
// exact sources already in the failover stack are omitted; a person may attach
// another source from the same playlist as an explicit backup.
func (q *ExperienceQueries) GetMatchSuggestions(ctx context.Context, channelID int64, limit int) ([]models.MatchSuggestion, int64, error) {
	if limit < 1 {
		return []models.MatchSuggestion{}, 0, nil
	}
	if limit > 5 {
		limit = 5
	}

	target := struct {
		Name  string  `db:"name"`
		TVGID *string `db:"tvgid"`
	}{}
	if err := q.GetContext(ctx, &target, `SELECT name, tvgid FROM templatechannel WHERE id = ?`, channelID); err != nil {
		return nil, 0, err
	}

	candidates := []sourceMatchCandidate{}
	if err := q.SelectContext(ctx, &candidates, `SELECT pc.id AS source_channel_id,
		COALESCE(NULLIF(pc.tvg_name, ''), pc.title) AS source_name, pc.title, pc.tvg_id,
		pc.tvg_logo AS logo_url, pg.id AS group_id, pg.name AS group_name,
		p.id AS playlist_id, p.name AS playlist_name
		FROM playlistchannel pc
		JOIN playlistgroup pg ON pg.id = pc.group_id
		JOIN playlist p ON p.id = pg.playlist_id
		WHERE pc.enabled = true AND pg.enabled = true
		AND NOT EXISTS (
			SELECT 1 FROM templatechannelitem attached
			WHERE attached.channel_id = ? AND attached.playlist_channel_id = pc.id
		)
		ORDER BY pc.id`, channelID); err != nil {
		return nil, 0, err
	}
	for index := range candidates {
		if err := revealOptionalProviderValue(&candidates[index].LogoURL); err != nil {
			return nil, 0, err
		}
	}

	rejections := []struct {
		PlaylistID int64  `db:"playlist_id"`
		TVGID      string `db:"tvg_id_norm"`
		Name       string `db:"name_norm"`
	}{}
	if err := q.SelectContext(ctx, &rejections, `SELECT playlist_id, tvg_id_norm, name_norm
		FROM channelmatchrejection WHERE channel_id = ?`, channelID); err != nil {
		return nil, 0, err
	}
	rejected := func(candidate sourceMatchCandidate) bool {
		tvgID := ""
		if candidate.TVGID != nil {
			tvgID = channelmatch.NormalizeTvgID(*candidate.TVGID)
		}
		name := channelmatch.ParseName(candidate.Title).Canonical
		for _, rejection := range rejections {
			if rejection.PlaylistID == candidate.PlaylistID &&
				((rejection.TVGID != "" && rejection.TVGID == tvgID) || rejection.Name == name) {
				return true
			}
		}
		return false
	}

	eligible := make([]sourceMatchCandidate, 0, len(candidates))
	rankCandidates := make([]channelmatch.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if rejected(candidate) {
			continue
		}
		eligible = append(eligible, candidate)
		rankCandidates = append(rankCandidates, channelmatch.Candidate{ID: candidate.SourceChannelID, Name: candidate.Title})
	}

	rankedByID := make(map[int64]channelmatch.Result, len(eligible))
	for _, result := range channelmatch.RankSuggestions(target.Name, rankCandidates, len(rankCandidates)) {
		rankedByID[result.Candidate.ID] = result
	}
	targetTVGID := ""
	if target.TVGID != nil {
		targetTVGID = channelmatch.NormalizeTvgID(*target.TVGID)
	}

	results := make([]models.MatchSuggestion, 0, len(eligible))
	for _, candidate := range eligible {
		nameResult, nameMatched := rankedByID[candidate.SourceChannelID]
		exactTVGID := targetTVGID != "" && candidate.TVGID != nil &&
			channelmatch.NormalizeTvgID(*candidate.TVGID) == targetTVGID
		suggestion := candidate.MatchSuggestion
		if exactTVGID {
			suggestion.Score = 1
			suggestion.Method = channelmatch.MethodExactTvgID
			if nameMatched && nameResult.Method == channelmatch.MethodExactName {
				suggestion.Method = channelmatch.MethodTvgIDName
			}
		} else {
			suggestion.Score = nameResult.Score
			suggestion.Method = nameResult.Method
		}
		results = append(results, suggestion)
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].SourceChannelID < results[j].SourceChannelID
		}
		return results[i].Score > results[j].Score
	})
	for index := range results {
		if index+1 < len(results) {
			runnerUp := results[index+1].Score
			results[index].RunnerUpScore = &runnerUp
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, int64(len(eligible)), nil
}

func (q *ExperienceQueries) AttachManualMatch(ctx context.Context, channelID, sourceChannelID int64) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var channelExists, sourceExists bool
		if err := tx.GetContext(ctx, &channelExists, `SELECT EXISTS(SELECT 1 FROM templatechannel WHERE id = ?)`, channelID); err != nil {
			return err
		}
		if err := tx.GetContext(ctx, &sourceExists, `SELECT EXISTS(SELECT 1 FROM playlistchannel WHERE id = ?)`, sourceChannelID); err != nil {
			return err
		}
		if !channelExists || !sourceExists {
			return sql.ErrNoRows
		}

		var existing bool
		if err := tx.GetContext(ctx, &existing, `SELECT EXISTS(SELECT 1 FROM templatechannelitem WHERE channel_id = ? AND playlist_channel_id = ?)`, channelID, sourceChannelID); err != nil {
			return err
		}
		if existing {
			if _, err := tx.ExecContext(ctx, `UPDATE templatechannelitem
				SET match_method = ?, match_score = 1, runner_up_score = NULL,
					matcher_version = ?, manual_locked = true
				WHERE channel_id = ? AND playlist_channel_id = ?`,
				channelmatch.MethodManual, channelmatch.CurrentVersion, channelID, sourceChannelID); err != nil {
				return err
			}
		} else {
			var order int64
			if err := tx.GetContext(ctx, &order, `SELECT COALESCE(MAX(orderr), 0) + 1 FROM templatechannelitem WHERE channel_id = ?`, channelID); err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, `INSERT INTO templatechannelitem (
				channel_id, playlist_channel_id, orderr, match_method, match_score,
				runner_up_score, matcher_version, manual_locked
			) VALUES (?, ?, ?, ?, 1, NULL, ?, true)`,
				channelID, sourceChannelID, order, channelmatch.MethodManual, channelmatch.CurrentVersion)
			if err != nil {
				return err
			}
			if affected, err := result.RowsAffected(); err != nil || affected == 0 {
				if err != nil {
					return err
				}
				return fmt.Errorf("source variant was not attached")
			}
		}

		identity := struct {
			TVGID      *string `db:"tvg_id"`
			Name       string  `db:"title"`
			PlaylistID int64   `db:"playlist_id"`
		}{}
		if err := tx.GetContext(ctx, &identity, `SELECT pc.tvg_id, pc.title, pg.playlist_id
			FROM playlistchannel pc JOIN playlistgroup pg ON pg.id = pc.group_id
			WHERE pc.id = ?`, sourceChannelID); err != nil {
			return err
		}
		tvgID := ""
		if identity.TVGID != nil {
			tvgID = channelmatch.NormalizeTvgID(*identity.TVGID)
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM channelmatchrejection
			WHERE channel_id = ? AND playlist_id = ?
			AND ((tvg_id_norm <> '' AND tvg_id_norm = ?) OR name_norm = ?)`,
			channelID, identity.PlaylistID, tvgID, channelmatch.ParseName(identity.Name).Canonical)
		return err
	})
}

func (q *ExperienceQueries) MoveMatchVariant(ctx context.Context, channelID, sourceChannelID int64, beforeID, afterID *int64) error {
	if (beforeID == nil) == (afterID == nil) {
		return fmt.Errorf("exactly one of before_id or after_id is required")
	}
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		ids := []int64{}
		if err := tx.SelectContext(ctx, &ids, `SELECT playlist_channel_id FROM templatechannelitem WHERE channel_id = ? ORDER BY orderr, id`, channelID); err != nil {
			return err
		}
		targetID := beforeID
		if targetID == nil {
			targetID = afterID
		}
		if sourceChannelID == *targetID {
			return fmt.Errorf("a source variant cannot be moved relative to itself")
		}
		ordered := make([]int64, 0, len(ids))
		sourceFound, targetFound := false, false
		for _, id := range ids {
			if id == sourceChannelID {
				sourceFound = true
				continue
			}
			if id == *targetID {
				targetFound = true
			}
			ordered = append(ordered, id)
		}
		if !sourceFound || !targetFound {
			return sql.ErrNoRows
		}
		insertAt := 0
		for index, id := range ordered {
			if id == *targetID {
				insertAt = index
				if afterID != nil {
					insertAt++
				}
				break
			}
		}
		ordered = append(ordered, 0)
		copy(ordered[insertAt+1:], ordered[insertAt:])
		ordered[insertAt] = sourceChannelID
		for index, id := range ordered {
			if _, err := tx.ExecContext(ctx, `UPDATE templatechannelitem SET orderr = ? WHERE channel_id = ? AND playlist_channel_id = ?`, index+1, channelID, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (q *ExperienceQueries) GetMatchRejections(ctx context.Context, channelID int64) ([]models.MatchRejection, error) {
	rows := []models.MatchRejection{}
	err := q.SelectContext(ctx, &rows, `SELECT r.id, r.channel_id, r.playlist_id, p.name AS playlist_name,
		r.tvg_id_norm, r.name_norm, r.created_at
		FROM channelmatchrejection r JOIN playlist p ON p.id = r.playlist_id
		WHERE r.channel_id = ? ORDER BY r.created_at DESC, r.id DESC LIMIT 100`, channelID)
	if err != nil || len(rows) == 0 {
		return rows, err
	}

	type currentRejectedSource struct {
		ID         int64   `db:"id"`
		Name       string  `db:"name"`
		Title      string  `db:"title"`
		TVGID      *string `db:"tvg_id"`
		LogoURL    *string `db:"logo_url"`
		PlaylistID int64   `db:"playlist_id"`
	}
	candidates := []currentRejectedSource{}
	if err := q.SelectContext(ctx, &candidates, `SELECT pc.id,
		COALESCE(NULLIF(pc.tvg_name, ''), pc.title) AS name, pc.title, pc.tvg_id,
		pc.tvg_logo AS logo_url, pg.playlist_id
		FROM playlistchannel pc
		JOIN playlistgroup pg ON pg.id = pc.group_id
		WHERE pc.enabled = true AND pg.enabled = true
		AND EXISTS (
			SELECT 1 FROM channelmatchrejection r
			WHERE r.channel_id = ? AND r.playlist_id = pg.playlist_id
		)
		AND NOT EXISTS (
			SELECT 1 FROM templatechannelitem attached
			WHERE attached.channel_id = ? AND attached.playlist_channel_id = pc.id
		)
		ORDER BY pc.id`, channelID, channelID); err != nil {
		return nil, err
	}
	for index := range candidates {
		if err := revealOptionalProviderValue(&candidates[index].LogoURL); err != nil {
			return nil, err
		}
	}

	for index := range rows {
		matches := make([]currentRejectedSource, 0, 1)
		for _, candidate := range candidates {
			if candidate.PlaylistID != rows[index].PlaylistID {
				continue
			}
			tvgID := ""
			if candidate.TVGID != nil {
				tvgID = channelmatch.NormalizeTvgID(*candidate.TVGID)
			}
			if (rows[index].TVGID != "" && rows[index].TVGID == tvgID) ||
				rows[index].Name == channelmatch.ParseName(candidate.Title).Canonical {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 1 {
			rows[index].SourceChannelID = &matches[0].ID
			rows[index].SourceName = &matches[0].Name
			rows[index].LogoURL = matches[0].LogoURL
		}
	}
	return rows, nil
}

func (q *ExperienceQueries) DeleteMatchRejection(ctx context.Context, channelID, rejectionID int64) (bool, error) {
	result, err := q.ExecContext(ctx, `DELETE FROM channelmatchrejection WHERE id = ? AND channel_id = ?`, rejectionID, channelID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (q *ExperienceQueries) GetStudioOverview(ctx context.Context) (*models.StudioOverview, error) {
	row := &models.StudioOverview{}
	query := `WITH match_health AS (
		SELECT
			(SELECT COUNT(*) FROM templatechannel tc WHERE NOT EXISTS (
				SELECT 1 FROM templatechannelitem tci WHERE tci.channel_id = tc.id
			)) AS unmatched_count,
			(SELECT COUNT(*) FROM templatechannel tc WHERE
				NOT EXISTS (SELECT 1 FROM templatechannelitem locked_item WHERE locked_item.channel_id = tc.id AND locked_item.manual_locked = 1)
				AND EXISTS (SELECT 1 FROM templatechannelitem review_item WHERE review_item.channel_id = tc.id AND review_item.manual_locked = 0
					AND review_item.match_score IS NOT NULL AND review_item.match_score < 0.82)
			) AS low_confidence_count
	)
	SELECT
		(SELECT COUNT(*) FROM template) AS lineup_count,
		(SELECT COUNT(*) FROM playlist) AS source_count,
		(SELECT COUNT(*) FROM playlistchannel) AS source_channel_count,
		(SELECT COUNT(*) FROM templatechannel) AS lineup_channel_count,
		(SELECT COUNT(*) FROM templatechannel WHERE tvgid IS NOT NULL AND tvgid != '') AS mapped_channel_count,
		match_health.unmatched_count + match_health.low_confidence_count AS review_count,
		match_health.low_confidence_count,
		match_health.unmatched_count,
		CASE WHEN (SELECT COUNT(*) FROM templatechannel) = 0 THEN 0 ELSE ROUND(100.0 * (SELECT COUNT(*) FROM templatechannel WHERE tvgid IS NOT NULL AND tvgid != '') / (SELECT COUNT(*) FROM templatechannel), 1) END AS epg_coverage,
		MAX((SELECT COUNT(*) FROM logo) - 1, 0) AS logo_count
		FROM match_health`
	if err := q.GetContext(ctx, row, query); err != nil {
		return nil, err
	}
	duplicates, err := q.loadDuplicateTVGIDReviews(ctx, nil, false)
	if err != nil {
		return nil, err
	}
	reviewChannelIDs := []int64{}
	if err := q.SelectContext(ctx, &reviewChannelIDs, `SELECT tc.id FROM templatechannel tc WHERE
		NOT EXISTS (SELECT 1 FROM templatechannelitem health_item WHERE health_item.channel_id = tc.id)
		OR (NOT EXISTS (SELECT 1 FROM templatechannelitem locked_item WHERE locked_item.channel_id = tc.id AND locked_item.manual_locked = 1)
			AND EXISTS (SELECT 1 FROM templatechannelitem review_item WHERE review_item.channel_id = tc.id AND review_item.manual_locked = 0
				AND review_item.match_score IS NOT NULL AND review_item.match_score < 0.82))`); err != nil {
		return nil, err
	}
	reviewSet := map[int64]bool{}
	for _, id := range reviewChannelIDs {
		reviewSet[id] = true
	}
	duplicateSet := map[int64]bool{}
	duplicateIssues := map[string]bool{}
	for _, duplicate := range duplicates {
		issueKey := channelmatch.NormalizeTvgID(duplicate.TVGID) + "\x00" + duplicateTVGSignature(duplicate.Channels)
		duplicateIssues[issueKey] = true
		for _, channel := range duplicate.Channels {
			reviewSet[channel.ID] = true
			duplicateSet[channel.ID] = true
		}
	}
	row.DuplicateTVGIDCount = int64(len(duplicateIssues))
	row.DuplicateTVGIDChannelCount = int64(len(duplicateSet))
	row.ReviewCount = int64(len(reviewSet))
	return row, nil
}

func (q *ExperienceQueries) GetCoverageSummary(ctx context.Context) (*models.CoverageSummary, error) {
	row := &models.CoverageSummary{}
	query := `SELECT
		(SELECT COUNT(*) FROM epgchannel) AS epg_channel_count,
		(SELECT COUNT(*) FROM epgprogramme) AS programme_count,
		(SELECT COUNT(*) FROM templatechannel WHERE tvgid IS NOT NULL AND tvgid != '') AS mapped_channel_count,
		(SELECT COUNT(*) FROM templatechannel) AS lineup_channel_count,
		CASE WHEN (SELECT COUNT(*) FROM templatechannel) = 0 THEN 0 ELSE ROUND(100.0 * (SELECT COUNT(*) FROM templatechannel WHERE tvgid IS NOT NULL AND tvgid != '') / (SELECT COUNT(*) FROM templatechannel), 1) END AS coverage`
	if err := q.GetContext(ctx, row, query); err != nil {
		return nil, err
	}
	row.UnmappedEPGIDs = []string{}
	if err := q.SelectContext(ctx, &row.UnmappedEPGIDs, `SELECT ec.channelid FROM epgchannel ec WHERE NOT EXISTS (SELECT 1 FROM templatechannel tc WHERE tc.tvgid = ec.channelid) ORDER BY LOWER(ec.channelid) LIMIT 250`); err != nil {
		return nil, err
	}
	row.ChannelsWithoutTVGID = []string{}
	if err := q.SelectContext(ctx, &row.ChannelsWithoutTVGID, `SELECT name FROM templatechannel WHERE tvgid IS NULL OR tvgid = '' ORDER BY LOWER(name) LIMIT 250`); err != nil {
		return nil, err
	}
	return row, nil
}

func (q *ExperienceQueries) GetJobs(ctx context.Context, limit int) ([]models.OperationJob, error) {
	rows := []models.OperationJob{}
	err := q.SelectContext(ctx, &rows, `SELECT id, kind, resource, resource_id, status, progress, message, error_code, created_at, updated_at, finished_at FROM operation_job ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	return rows, err
}

func (q *ExperienceQueries) GetJob(ctx context.Context, id int64) (*models.OperationJob, error) {
	row := &models.OperationJob{}
	err := q.GetContext(ctx, row, `SELECT id, kind, resource, resource_id, status, progress, message, error_code, created_at, updated_at, finished_at FROM operation_job WHERE id = ?`, id)
	return row, err
}

func (q *ExperienceQueries) CreateJob(ctx context.Context, kind, resource string, resourceID *int64, message string) (*models.OperationJob, error) {
	result, err := q.ExecContext(ctx, `INSERT INTO operation_job (kind, resource, resource_id, status, progress, message) VALUES (?, ?, ?, 'queued', 0, ?)`, kind, resource, resourceID, message)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return q.GetJob(ctx, id)
}

func (q *ExperienceQueries) UpdateJob(ctx context.Context, id int64, status string, progress int, message, errorCode string) error {
	finished := any(nil)
	if status == "succeeded" || status == "failed" || status == "cancelled" {
		finished = time.Now().UTC()
	}
	_, err := q.ExecContext(ctx, `UPDATE operation_job
		SET status = ?, progress = ?, message = ?, error_code = ?, updated_at = datetime('now'), finished_at = ?
		WHERE id = ?`, status, progress, message, errorCode, finished, id)
	return err
}

// FailIncompleteJobs closes jobs whose in-memory workers were lost when the
// process stopped. Jobs are not resumable, so leaving them queued or running
// after startup would make the activity feed report work that no longer exists.
func (q *ExperienceQueries) FailIncompleteJobs(ctx context.Context) (int64, error) {
	result, err := q.ExecContext(ctx, `UPDATE operation_job
		SET status = 'failed',
			progress = 100,
			message = 'Interrupted because Xivi restarted before the operation finished. Run it again.',
			error_code = 'job_interrupted',
			updated_at = datetime('now'),
			finished_at = datetime('now')
		WHERE status IN ('queued', 'running')`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (q *ExperienceQueries) MoveWorkspaceGroup(ctx context.Context, lineupID, groupID int64, beforeID, afterID *int64) error {
	if (beforeID == nil) == (afterID == nil) {
		return fmt.Errorf("exactly one of before_id or after_id is required")
	}
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var lineupExists bool
		if err := tx.GetContext(ctx, &lineupExists, `SELECT EXISTS(SELECT 1 FROM template WHERE id = ?)`, lineupID); err != nil {
			return err
		}
		if !lineupExists {
			return ErrLineupNotFound
		}
		ids := []int64{}
		if err := tx.SelectContext(ctx, &ids, `SELECT group_id FROM template_group_item WHERE template_id = ? ORDER BY orderr, group_id`, lineupID); err != nil {
			return err
		}
		targetID := beforeID
		if targetID == nil {
			targetID = afterID
		}
		if groupID == *targetID {
			return fmt.Errorf("a group cannot be moved relative to itself")
		}
		ordered := make([]int64, 0, len(ids))
		sourceFound, targetFound := false, false
		for _, id := range ids {
			if id == groupID {
				sourceFound = true
				continue
			}
			if id == *targetID {
				targetFound = true
			}
			ordered = append(ordered, id)
		}
		if !sourceFound || !targetFound {
			return ErrStudioGroupNotFound
		}
		insertAt := 0
		for index, id := range ordered {
			if id == *targetID {
				insertAt = index
				if afterID != nil {
					insertAt++
				}
				break
			}
		}
		ordered = append(ordered, 0)
		copy(ordered[insertAt+1:], ordered[insertAt:])
		ordered[insertAt] = groupID
		for index, id := range ordered {
			if _, err := tx.ExecContext(ctx, `UPDATE template_group_item SET orderr = ? WHERE template_id = ? AND group_id = ?`, index+1, lineupID, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (q *ExperienceQueries) MoveWorkspaceChannel(ctx context.Context, groupID, channelID int64, beforeID, afterID *int64) error {
	if (beforeID == nil) == (afterID == nil) {
		return fmt.Errorf("exactly one of before_id or after_id is required")
	}
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		ids := []int64{}
		if err := tx.SelectContext(ctx, &ids, `SELECT channel_id FROM template_group_channel WHERE group_id = ? ORDER BY orderr, channel_id`, groupID); err != nil {
			return err
		}
		targetID := beforeID
		if targetID == nil {
			targetID = afterID
		}
		if channelID == *targetID {
			return fmt.Errorf("a channel cannot be moved relative to itself")
		}
		ordered := make([]int64, 0, len(ids))
		sourceFound, targetFound := false, false
		for _, id := range ids {
			if id == channelID {
				sourceFound = true
				continue
			}
			if id == *targetID {
				targetFound = true
			}
			ordered = append(ordered, id)
		}
		if !sourceFound || !targetFound {
			return sql.ErrNoRows
		}
		insertAt := 0
		for index, id := range ordered {
			if id == *targetID {
				insertAt = index
				if afterID != nil {
					insertAt++
				}
				break
			}
		}
		ordered = append(ordered, 0)
		copy(ordered[insertAt+1:], ordered[insertAt:])
		ordered[insertAt] = channelID
		for index, id := range ordered {
			if _, err := tx.ExecContext(ctx, `UPDATE template_group_channel SET orderr = ? WHERE group_id = ? AND channel_id = ?`, index+1, groupID, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// BatchAddWorkspaceChannels creates canonical lineup channels from source
// channels and attaches the source as a manually locked variant in one
// transaction. The caller supplies UUIDs so UUID generation stays outside the
// query layer and the operation remains deterministic in tests.
func (q *ExperienceQueries) BatchAddWorkspaceChannels(ctx context.Context, groupID int64, sourceChannelIDs []int64, uuids []string) ([]int64, error) {
	if len(sourceChannelIDs) == 0 || len(sourceChannelIDs) != len(uuids) {
		return nil, fmt.Errorf("source channel ids and uuids are required")
	}
	created := make([]int64, 0, len(sourceChannelIDs))
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var groupExists bool
		if err := tx.GetContext(ctx, &groupExists, `SELECT EXISTS(SELECT 1 FROM templategroup WHERE id = ?)`, groupID); err != nil {
			return err
		}
		if !groupExists {
			return sql.ErrNoRows
		}
		var position int64
		if err := tx.GetContext(ctx, &position, `SELECT COALESCE(MAX(orderr), 0) FROM template_group_channel WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		type sourceIdentity struct {
			Name  string  `db:"name"`
			TVGID *string `db:"tvg_id"`
		}
		seen := map[int64]bool{}
		for index, sourceID := range sourceChannelIDs {
			if sourceID < 1 || seen[sourceID] || strings.TrimSpace(uuids[index]) == "" {
				return fmt.Errorf("source channel selection contains an invalid or duplicate id")
			}
			seen[sourceID] = true
			source := sourceIdentity{}
			if err := tx.GetContext(ctx, &source, `SELECT COALESCE(NULLIF(title, ''), NULLIF(tvg_name, ''), 'Untitled channel') AS name, tvg_id FROM playlistchannel WHERE id = ?`, sourceID); err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, `INSERT INTO templatechannel (name, tvgid, logoid, uuid) VALUES (?, ?, 0, ?)`, source.Name, source.TVGID, uuids[index])
			if err != nil {
				return err
			}
			channelID, err := result.LastInsertId()
			if err != nil {
				return err
			}
			position++
			if _, err := tx.ExecContext(ctx, `INSERT INTO template_group_channel (group_id, channel_id, orderr) VALUES (?, ?, ?)`, groupID, channelID, position); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO templatechannelitem (channel_id, playlist_channel_id, orderr, match_method, match_score, matcher_version, manual_locked) VALUES (?, ?, 1, 'manual', 1, 2, true)`, channelID, sourceID); err != nil {
				return err
			}
			created = append(created, channelID)
		}
		return nil
	})
	return created, err
}

// BatchRemoveWorkspaceChannels removes canonical channels only after proving
// every requested channel belongs to the addressed group. Foreign-key cascades
// remove their memberships, variants, vectors, and rejection history.
func (q *ExperienceQueries) BatchRemoveWorkspaceChannels(ctx context.Context, groupID int64, channelIDs []int64) error {
	if len(channelIDs) == 0 {
		return fmt.Errorf("select at least one channel")
	}
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		seen := map[int64]bool{}
		affectedGroups := map[int64]bool{groupID: true}
		for _, channelID := range channelIDs {
			if channelID < 1 || seen[channelID] {
				return fmt.Errorf("channel selection contains an invalid or duplicate id")
			}
			seen[channelID] = true
			var exists bool
			if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM template_group_channel WHERE group_id = ? AND channel_id = ?)`, groupID, channelID); err != nil {
				return err
			}
			if !exists {
				return sql.ErrNoRows
			}
			groupIDs := []int64{}
			if err := tx.SelectContext(ctx, &groupIDs, `SELECT group_id FROM template_group_channel WHERE channel_id = ?`, channelID); err != nil {
				return err
			}
			for _, affectedGroupID := range groupIDs {
				affectedGroups[affectedGroupID] = true
			}
		}
		for _, channelID := range channelIDs {
			if _, err := tx.ExecContext(ctx, `DELETE FROM templatechannel WHERE id = ?`, channelID); err != nil {
				return err
			}
		}
		for affectedGroupID := range affectedGroups {
			if err := reindexWorkspaceGroup(ctx, tx, affectedGroupID); err != nil {
				return err
			}
		}
		return nil
	})
}

// BatchMoveWorkspaceChannels moves a selected block using a stable before/after
// anchor. Omitting both anchors appends the block to the destination group.
func (q *ExperienceQueries) BatchMoveWorkspaceChannels(ctx context.Context, sourceGroupID, targetGroupID int64, channelIDs []int64, beforeID, afterID *int64) error {
	if len(channelIDs) == 0 || targetGroupID < 1 {
		return fmt.Errorf("channels and target_group_id are required")
	}
	if beforeID != nil && afterID != nil {
		return fmt.Errorf("only one of before_id or after_id may be supplied")
	}
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var targetExists bool
		if err := tx.GetContext(ctx, &targetExists, `SELECT EXISTS(SELECT 1 FROM templategroup WHERE id = ?)`, targetGroupID); err != nil {
			return err
		}
		if !targetExists {
			return sql.ErrNoRows
		}
		sourceOrder, err := workspaceGroupOrder(ctx, tx, sourceGroupID)
		if err != nil {
			return err
		}
		selected := make(map[int64]bool, len(channelIDs))
		for _, channelID := range channelIDs {
			if channelID < 1 || selected[channelID] {
				return fmt.Errorf("channel selection contains an invalid or duplicate id")
			}
			selected[channelID] = true
		}
		for channelID := range selected {
			if !containsInt64(sourceOrder, channelID) {
				return sql.ErrNoRows
			}
		}
		destinationOrder := sourceOrder
		if sourceGroupID != targetGroupID {
			destinationOrder, err = workspaceGroupOrder(ctx, tx, targetGroupID)
			if err != nil {
				return err
			}
		}
		destinationOrder = withoutWorkspaceChannels(destinationOrder, selected)
		anchor := beforeID
		if anchor == nil {
			anchor = afterID
		}
		insertAt := len(destinationOrder)
		if anchor != nil {
			if selected[*anchor] {
				return fmt.Errorf("a moved channel cannot be its own anchor")
			}
			insertAt = -1
			for index, channelID := range destinationOrder {
				if channelID == *anchor {
					insertAt = index
					if afterID != nil {
						insertAt++
					}
					break
				}
			}
			if insertAt < 0 {
				return sql.ErrNoRows
			}
		}
		finalOrder := make([]int64, 0, len(destinationOrder)+len(channelIDs))
		finalOrder = append(finalOrder, destinationOrder[:insertAt]...)
		finalOrder = append(finalOrder, channelIDs...)
		finalOrder = append(finalOrder, destinationOrder[insertAt:]...)
		if sourceGroupID != targetGroupID {
			for _, channelID := range channelIDs {
				if _, err := tx.ExecContext(ctx, `DELETE FROM template_group_channel WHERE group_id = ? AND channel_id = ?`, sourceGroupID, channelID); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO template_group_channel (group_id, channel_id, orderr) VALUES (?, ?, 0)`, targetGroupID, channelID); err != nil {
					return err
				}
			}
			if err := reindexWorkspaceGroup(ctx, tx, sourceGroupID); err != nil {
				return err
			}
		}
		for index, channelID := range finalOrder {
			if _, err := tx.ExecContext(ctx, `UPDATE template_group_channel SET orderr = ? WHERE group_id = ? AND channel_id = ?`, index+1, targetGroupID, channelID); err != nil {
				return err
			}
		}
		return nil
	})
}

func workspaceGroupOrder(ctx context.Context, tx *sqlx.Tx, groupID int64) ([]int64, error) {
	ids := []int64{}
	err := tx.SelectContext(ctx, &ids, `SELECT channel_id FROM template_group_channel WHERE group_id = ? ORDER BY orderr, channel_id`, groupID)
	return ids, err
}

func reindexWorkspaceGroup(ctx context.Context, tx *sqlx.Tx, groupID int64) error {
	ids, err := workspaceGroupOrder(ctx, tx, groupID)
	if err != nil {
		return err
	}
	for index, channelID := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE template_group_channel SET orderr = ? WHERE group_id = ? AND channel_id = ?`, index+1, groupID, channelID); err != nil {
			return err
		}
	}
	return nil
}

func withoutWorkspaceChannels(ids []int64, selected map[int64]bool) []int64 {
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !selected[id] {
			result = append(result, id)
		}
	}
	return result
}

func containsInt64(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

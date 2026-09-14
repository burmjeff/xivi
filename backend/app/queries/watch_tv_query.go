package queries

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"xivi/backend/app/models"
)

// Current category names are metadata, not a full-catalog schedule download.
// The caller uses the same lineup authorization as the channel/guide routes.
func (q *SecurityQueries) WatchOnNowCategories(ctx context.Context, lineupID int64) ([]string, error) {
	now := epgQueryTime(time.Now().UTC())
	values := []string{}
	err := q.SelectContext(ctx, &values, `SELECT DISTINCT p.categories FROM epgprogramme p
		JOIN templatechannel tc ON tc.tvgid=p.channel
		WHERE p.start<=? AND p.stop>? AND p.categories IS NOT NULL AND p.categories!=''
		AND EXISTS(SELECT 1 FROM template_group_channel tgc JOIN template_group_item tgi ON tgi.group_id=tgc.group_id
			WHERE tgc.channel_id=tc.id AND tgi.template_id=?)
		AND EXISTS(SELECT 1 FROM templatechannelitem ti WHERE ti.channel_id=tc.id)`, now, now, lineupID)
	if err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for _, value := range values {
		for _, category := range strings.Split(value, ",") {
			category = strings.TrimSpace(category)
			if category != "" && len(category) <= 80 {
				seen[strings.ToLower(category)] = category
			}
		}
	}
	items := make([]string, 0, len(seen))
	for _, category := range seen {
		items = append(items, category)
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i]) < strings.ToLower(items[j]) })
	return items, nil
}

type WatchSearchRow struct {
	Kind        string     `db:"kind" json:"kind"`
	LineupID    int64      `db:"lineup_id" json:"lineup_id"`
	ChannelID   int64      `db:"channel_id" json:"channel_id"`
	ProgrammeID int64      `db:"programme_id" json:"programme_id"`
	Title       string     `db:"title" json:"title"`
	Start       *time.Time `db:"start" json:"start"`
	End         *time.Time `db:"end" json:"end"`
}

func (q *SecurityQueries) SearchWatch(ctx context.Context, principal models.SessionPrincipal, lineupID *int64, term string, limit, offset int) ([]WatchSearchRow, int64, error) {
	filter := ` WHERE EXISTS(SELECT 1 FROM templatechannelitem ti WHERE ti.channel_id=tc.id)`
	args := []any{}
	if !principal.IsAdmin() {
		filter += ` AND EXISTS(SELECT 1 FROM user_lineup ul WHERE ul.user_id=? AND ul.lineup_id=tgi.template_id)`
		args = append(args, principal.UserID)
	}
	if lineupID != nil {
		filter += ` AND tgi.template_id=?`
		args = append(args, *lineupID)
	}
	base := ` FROM templatechannel tc JOIN template_group_channel tgc ON tgc.channel_id=tc.id JOIN template_group_item tgi ON tgi.group_id=tgc.group_id`
	like := "%" + strings.ToLower(term) + "%"
	now := time.Now().UTC()
	cte := `WITH results AS (SELECT DISTINCT 'channel' AS kind,tgi.template_id AS lineup_id,tc.id AS channel_id,0 AS programme_id,tc.name AS title,NULL AS start,NULL AS end` + base + filter + ` AND (LOWER(tc.name) LIKE ? OR LOWER(COALESCE(tc.tvgid,'')) LIKE ?)
	UNION ALL SELECT DISTINCT 'programme',tgi.template_id,tc.id,p.id,COALESCE(p."title.value",''),p.start,p.stop` + base + ` JOIN epgprogramme p ON p.channel=tc.tvgid` + filter + ` AND LOWER(p."title.value") LIKE ? AND p.stop>? AND p.start<?)`
	all := append([]any{}, args...)
	all = append(all, like, like)
	all = append(all, args...)
	all = append(all, like, epgQueryTime(now), epgQueryTime(now.Add(7*24*time.Hour)))
	var total int64
	if err := q.GetContext(ctx, &total, cte+` SELECT COUNT(*) FROM results`, all...); err != nil {
		return nil, 0, err
	}
	rows := []WatchSearchRow{}
	page := append(append([]any{}, all...), limit, offset)
	err := q.SelectContext(ctx, &rows, cte+` SELECT * FROM results ORDER BY kind,start,title,lineup_id,channel_id,programme_id LIMIT ? OFFSET ?`, page...)
	return rows, total, err
}

// Validate in small parameter batches, without one database query per channel.
func (q *SecurityQueries) ValidateViewerChannelKeys(ctx context.Context, p models.SessionPrincipal, keys [][2]int64) (bool, error) {
	for start := 0; start < len(keys); start += 100 {
		batch := keys[start:min(start+100, len(keys))]
		parts := make([]string, len(batch))
		args := []any{}
		for i, key := range batch {
			parts[i] = "(?,?)"
			args = append(args, key[0], key[1])
		}
		filter := ""
		if !p.IsAdmin() {
			filter = ` AND EXISTS(SELECT 1 FROM user_lineup ul WHERE ul.user_id=? AND ul.lineup_id=k.lineup_id)`
			args = append(args, p.UserID)
		}
		query := `WITH keys(lineup_id,channel_id) AS (VALUES ` + strings.Join(parts, ",") + `) SELECT COUNT(*) FROM keys k WHERE EXISTS(SELECT 1 FROM template_group_channel tgc JOIN template_group_item tgi ON tgi.group_id=tgc.group_id WHERE tgc.channel_id=k.channel_id AND tgi.template_id=k.lineup_id)` + filter
		var count int
		if err := q.GetContext(ctx, &count, query, args...); err != nil {
			return false, fmt.Errorf("validate viewer channels: %w", err)
		}
		if count != len(batch) {
			return false, nil
		}
	}
	return true, nil
}

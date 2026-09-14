package queries

import (
	"context"
	"testing"
	"time"
	"xivi/backend/app/models"
)

func TestWatchSearchIsTypedPaginatedAndLineupAuthorized(t *testing.T) {
	q, d, now := pairedTV(t)
	ctx := context.Background()
	q.MustExec(`INSERT INTO template(id,name) VALUES(1,'Allowed'),(2,'Forbidden')`)
	q.MustExec(`INSERT INTO user_lineup(user_id,lineup_id) VALUES(?,1)`, d.UserID)
	q.MustExec(`CREATE TABLE templatechannel(id INTEGER PRIMARY KEY,name TEXT,tvgid TEXT)`)
	q.MustExec(`CREATE TABLE templatechannelitem(id INTEGER PRIMARY KEY,channel_id INTEGER)`)
	q.MustExec(`CREATE TABLE template_group_channel(group_id INTEGER,channel_id INTEGER)`)
	q.MustExec(`CREATE TABLE template_group_item(group_id INTEGER,template_id INTEGER)`)
	q.MustExec(`CREATE TABLE epgprogramme(id INTEGER PRIMARY KEY,channel TEXT,"title.value" TEXT,start TIMESTAMP,stop TIMESTAMP)`)
	q.MustExec(`INSERT INTO templatechannel VALUES(1,'Sports One','sports-one'),(2,'Sports Private','sports-two')`)
	q.MustExec(`INSERT INTO templatechannelitem VALUES(1,1),(2,2)`)
	q.MustExec(`INSERT INTO template_group_channel VALUES(1,1),(2,2)`)
	q.MustExec(`INSERT INTO template_group_item VALUES(1,1),(2,2)`)
	q.MustExec(`INSERT INTO epgprogramme VALUES(1,'sports-one','Sports Tonight',?,?),(2,'sports-two','Sports Secret',?,?)`, epgQueryTime(now), epgQueryTime(now.Add(time.Hour)), epgQueryTime(now), epgQueryTime(now.Add(time.Hour)))
	q.MustExec(`ALTER TABLE epgprogramme ADD COLUMN categories TEXT`)
	q.MustExec(`UPDATE epgprogramme SET categories=CASE id WHEN 1 THEN 'Sports, Live' ELSE 'Private' END`)
	categories, err := q.WatchOnNowCategories(ctx, 1)
	if err != nil || len(categories) != 2 || categories[0] != "Live" || categories[1] != "Sports" {
		t.Fatalf("current categories: %v %v", categories, err)
	}
	p := models.SessionPrincipal{UserID: d.UserID, Role: models.RoleViewer, LineupIDs: []int64{1}}
	rows, total, err := q.SearchWatch(ctx, p, nil, "sports", 1, 0)
	if err != nil || total != 2 || len(rows) != 1 || rows[0].Kind != "channel" || rows[0].LineupID != 1 {
		t.Fatalf("channel page: %+v %d %v", rows, total, err)
	}
	rows, total, err = q.SearchWatch(ctx, p, nil, "sports", 1, 1)
	if err != nil || total != 2 || len(rows) != 1 || rows[0].Kind != "programme" || rows[0].Start == nil || rows[0].LineupID != 1 {
		t.Fatalf("programme page: %+v %d %v", rows, total, err)
	}
	valid, err := q.ValidateViewerChannelKeys(ctx, p, [][2]int64{{1, 1}})
	if err != nil || !valid {
		t.Fatalf("allowed preference key: %v", err)
	}
	valid, err = q.ValidateViewerChannelKeys(ctx, p, [][2]int64{{2, 2}})
	if err != nil || valid {
		t.Fatalf("forbidden preference key accepted: %v", err)
	}
}

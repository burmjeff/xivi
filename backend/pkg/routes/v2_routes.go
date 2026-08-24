package routes

import (
	"github.com/gofiber/fiber/v2"
	"xivi/backend/app/controllers"
)

// V2Routes is additive: public device, output and legacy JSON contracts remain
// registered separately and unchanged.
func V2Routes(a *fiber.App) {
	v2 := a.Group("/api/v2")
	v2.Get("/watch/lineups", controllers.V2WatchLineups)
	v2.Get("/watch/lineups/:lineup_id/channels", controllers.V2WatchChannels)
	v2.Get("/watch/lineups/:lineup_id/guide", controllers.V2WatchGuide)
	v2.Get("/watch/channels/:channel_id", controllers.V2WatchChannel)
	v2.Get("/search", controllers.V2Search)

	v2.Get("/studio/overview", controllers.V2StudioOverview)
	v2.Get("/studio/guide-data/coverage", controllers.V2StudioCoverage)
	v2.Get("/studio/lineups", controllers.V2StudioLineups)
	v2.Get("/studio/lineups/:lineup_id/groups", controllers.V2StudioLineupGroups)
	v2.Post("/studio/lineups/:lineup_id/groups", controllers.V2CreateStudioGroup)
	v2.Patch("/studio/lineups/:lineup_id/groups/:group_id/position", controllers.V2MoveStudioGroup)
	v2.Post("/studio/lineups/:lineup_id/source-groups/:source_group_id/copy", controllers.V2CopySourceGroupToLineup)
	v2.Get("/studio/source-groups", controllers.V2StudioSourceGroups)
	v2.Patch("/studio/groups/:group_id", controllers.V2UpdateStudioGroup)
	v2.Delete("/studio/groups/:group_id", controllers.V2DeleteStudioGroup)
	v2.Put("/studio/groups/:group_id/source-link", controllers.V2SetStudioGroupSourceLink)
	v2.Delete("/studio/groups/:group_id/source-link", controllers.V2DeleteStudioGroupSourceLink)
	v2.Post("/studio/groups/:group_id/sync", controllers.V2SyncStudioGroup)
	v2.Get("/studio/groups/:group_id/channels", controllers.V2StudioGroupChannels)
	v2.Post("/studio/groups/:group_id/channels/batch-add", controllers.V2BatchAddStudioChannels)
	v2.Post("/studio/groups/:group_id/source-groups/:source_group_id/add", controllers.V2AddSourceGroupToStudioGroup)
	v2.Post("/studio/groups/:group_id/channels/batch-remove", controllers.V2BatchRemoveStudioChannels)
	v2.Post("/studio/groups/:group_id/channels/batch-move", controllers.V2BatchMoveStudioChannels)
	v2.Patch("/studio/groups/:group_id/channels/:channel_id/position", controllers.V2MoveStudioChannel)
	v2.Patch("/studio/channels/:channel_id", controllers.V2UpdateStudioChannel)
	v2.Get("/studio/source-channels", controllers.V2StudioSourceChannels)
	v2.Patch("/studio/source-channels/enabled", controllers.V2SetStudioSourceChannelsEnabled)
	v2.Get("/studio/review", controllers.V2StudioReview)
	v2.Get("/studio/channels/:channel_id/matches", controllers.V2StudioReview)
	v2.Get("/studio/channels/:channel_id/suggestions", controllers.V2StudioMatchSuggestions)
	v2.Get("/studio/channels/:channel_id/rejections", controllers.V2StudioMatchRejections)
	v2.Post("/studio/channels/:channel_id/matches/:source_channel_id/accept", controllers.V2AcceptStudioMatch)
	v2.Post("/studio/channels/:channel_id/matches/:source_channel_id/reject", controllers.V2RejectStudioMatch)
	v2.Post("/studio/sources/:source_id/refresh", controllers.V2RefreshSource)
	v2.Post("/studio/guide-data/sources/:source_id/refresh", controllers.V2RefreshGuideSource)
	v2.Post("/studio/lineups/:lineup_id/publish", controllers.V2PublishLineup)

	v2.Get("/jobs", controllers.V2Jobs)
	v2.Get("/jobs/:job_id", controllers.V2Job)
	v2.Get("/events", controllers.V2Events)
}

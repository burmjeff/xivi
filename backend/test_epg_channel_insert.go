package main

import (
	"context"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

func main() {
	// Initialize database
	var err error
	database.Db, err = database.OpenDBConnection()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	// No need to close DB as it's managed by the application

	ctx := context.Background()

	// Create test channels with duplicate IDs
	channels := []models.EpgChannel{
		{
			ChannelId:   "TEST1",
			DisplayName: "Test Channel 1",
			Icon:        models.Icon{Src: "http://example.com/icon1.png"},
		},
		{
			ChannelId:   "TEST2",
			DisplayName: "Test Channel 2",
			Icon:        models.Icon{Src: "http://example.com/icon2.png"},
		},
		{
			ChannelId:   "TEST1", // Duplicate ID
			DisplayName: "Test Channel 1 Duplicate",
			Icon:        models.Icon{Src: "http://example.com/icon1_dup.png"},
		},
	}

	// First, try to insert all channels
	log.Info().Msg("First batch: Inserting channels including duplicates")
	err = database.Db.BatchCreateEpgChannels(ctx, channels)
	if err != nil {
		log.Info().Err(err).Msg("Expected error for duplicate channels (this is normal)")
	}

	// Now try to insert the same channels again
	log.Info().Msg("Second batch: Inserting the same channels again")
	err = database.Db.BatchCreateEpgChannels(ctx, channels)
	if err != nil {
		log.Info().Err(err).Msg("Expected error for duplicate channels (this is normal)")
	}

	// Fetch all channels to verify
	existingChannels, err := database.Db.GetEpgChannels(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch existing channels")
		return
	}

	log.Info().Int("count", len(*existingChannels)).Msg("Total channels in database")

	// Test the processChannels function
	log.Info().Msg("Testing processChannels function")
	testProcessChannels(ctx)

	log.Info().Msg("Test completed")
}

func testProcessChannels(ctx context.Context) {
	// Create test channels with unique IDs for this test
	channels := []models.EpgChannel{
		{
			ChannelId:   "TEST_UNIQUE_1",
			DisplayName: "Test Unique Channel 1",
			Icon:        models.Icon{Src: "http://example.com/icon_unique_1.png"},
		},
		{
			ChannelId:   "TEST_UNIQUE_2",
			DisplayName: "Test Unique Channel 2",
			Icon:        models.Icon{Src: "http://example.com/icon_unique_2.png"},
		},
	}

	// First, insert the channels individually to ensure they're created
	log.Info().Msg("Inserting test channels individually")
	for _, channel := range channels {
		_, err := database.Db.CreateEpgChannel(ctx, channel)
		if err != nil {
			log.Error().Err(err).Str("channelId", channel.ChannelId).Msg("Failed to create channel")
		} else {
			log.Info().Str("channelId", channel.ChannelId).Msg("Successfully created channel")
		}
	}

	// Now create a duplicate channel to test the handling
	duplicateChannel := models.EpgChannel{
		ChannelId:   "TEST_UNIQUE_1", // Duplicate of the first channel
		DisplayName: "Test Unique Channel 1 Updated",
		Icon:        models.Icon{Src: "http://example.com/icon_unique_1_updated.png"},
	}

	// Try to insert the duplicate
	log.Info().Msg("Trying to insert duplicate channel")
	_, err := database.Db.CreateEpgChannel(ctx, duplicateChannel)
	if err != nil {
		log.Info().Err(err).Str("channelId", duplicateChannel.ChannelId).Msg("Expected error for duplicate channel (this is normal)")
	}

	// Fetch all channels to verify
	existingChannels, err := database.Db.GetEpgChannels(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch existing channels")
		return
	}

	// Check if our test channels are in the database
	found := 0
	for _, existingChannel := range *existingChannels {
		if existingChannel.ChannelId == "TEST_UNIQUE_1" || existingChannel.ChannelId == "TEST_UNIQUE_2" {
			log.Info().Str("channelId", existingChannel.ChannelId).Str("displayName", existingChannel.DisplayName).Msg("Found test channel in database")
			found++
		}
	}

	log.Info().Int("foundCount", found).Int("expectedCount", 2).Msg("Test channels found in database")
}

package main

import (
	"context"
	"fmt"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/jetstream/pkg/models"
)

func (dsmt *DontShowMeThis) handlePost(ctx context.Context, event *models.Event, post *bsky.FeedPost) error {
	fmt.Println(post.Text)

	return nil
}

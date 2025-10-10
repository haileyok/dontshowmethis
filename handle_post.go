package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/jetstream/pkg/models"
)

func (dsmt *DontShowMeThis) handlePost(ctx context.Context, event *models.Event, post *bsky.FeedPost) error {
	if event == nil || event.Commit == nil {
		return nil
	}

	if post.Reply == nil {
		return nil
	}

	if post.Reply.Parent == nil {
		return errors.New("badly formatted reply ref (no parent)")
	}

	atUri, err := syntax.ParseATURI(post.Reply.Parent.Uri)
	if err != nil {
		return fmt.Errorf("failed to parse parent aturi: %w", err)
	}

	opDid := atUri.Authority().String()
	_, ok := dsmt.watchedOps[opDid]
	if !ok {
		return nil
	}

	uri := fmt.Sprintf("at://%s/%s/%s", event.Did, event.Commit.Collection, event.Commit.RKey)

	logger := dsmt.logger.With("opDid", opDid, "replyDid", event.Did, "uri", uri)

	logger.Info("ingested reply to watched op")

	if post.Text == "" {
		logger.Info("post contained no text, skipping")
		return nil
	}

	parent, err := dsmt.getPost(ctx, post.Reply.Parent.Uri)
	if err != nil {
		return fmt.Errorf("failed to get parent post: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	isBadFaith, err := dsmt.lmstudioc.GetIsBadFaith(ctx, parent.Text, post.Text)
	if err != nil {
		return fmt.Errorf("failed to check bad faith: %w", err)
	}

	if !isBadFaith {
		logger.Info("determined that reply was not bad faith")
		return nil
	}

	if err := dsmt.emitLabel(ctx, uri, LabelBadFaith); err != nil {
		return fmt.Errorf("failed to label post: %w", err)
	}

	logger.Info("determined that reply was bad faith and emitted label")

	return nil
}

package main

import (
	"context"
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

	var parentUri string

	if post.Reply != nil && post.Reply.Parent != nil {
		parentUri = post.Reply.Parent.Uri
	} else if post.Embed != nil && post.Embed.EmbedRecord != nil && post.Embed.EmbedRecord.Record != nil {
		parentUri = post.Embed.EmbedRecord.Record.Uri
	} else if post.Embed != nil && post.Embed.EmbedRecordWithMedia != nil && post.Embed.EmbedRecordWithMedia.Record != nil && post.Embed.EmbedRecordWithMedia.Record.Record != nil {
		parentUri = post.Embed.EmbedRecordWithMedia.Record.Record.Uri
	}

	if parentUri == "" {
		return nil
	}

	atUri, err := syntax.ParseATURI(parentUri)
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

	parent, err := dsmt.getPost(ctx, parentUri)
	if err != nil {
		return fmt.Errorf("failed to get parent post: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	results, err := dsmt.lmstudioc.GetIsBadFaith(ctx, parent.Text, post.Text)
	if err != nil {
		return fmt.Errorf("failed to check bad faith: %w", err)
	}

	if results.BadFaith {
		if err := dsmt.emitLabel(ctx, uri, LabelBadFaith); err != nil {
			return fmt.Errorf("failed to label post: %w", err)
		}
		logger.Info("determined that reply was bad faith and emitted label")
	}

	if results.OffTopic {
		if err := dsmt.emitLabel(ctx, uri, LabelOffTopic); err != nil {
			return fmt.Errorf("failed to label post: %w", err)
		}
		logger.Info("determined that reply was off topic and emitted label")
	}

	if results.Funny {
		if err := dsmt.emitLabel(ctx, uri, LabelFunny); err != nil {
			return fmt.Errorf("failed to label post: %w", err)
		}
		logger.Info("determined that reply was funny and emitted label")
	}

	return nil
}

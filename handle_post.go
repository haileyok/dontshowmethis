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
	_, isWatchedOp := dsmt.watchedOps[opDid]
	_, isWatchedLogOp := dsmt.watchedLogOps[opDid]
	if !isWatchedOp && !isWatchedLogOp {
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

	labels := []string{}
	if results.BadFaith {
		labels = append(labels, LabelBadFaith)
	}
	if results.OffTopic {
		labels = append(labels, LabelOffTopic)
	}
	if results.Funny {
		labels = append(labels, LabelFunny)
	}

	if len(labels) == 0 {
		if dsmt.logNoLabels && dsmt.db != nil {
			item := LogItem{
				ParentDid:  opDid,
				AuthorDid:  event.Did,
				ParentUri:  parentUri,
				AuthorUri:  uri,
				ParentText: parent.Text,
				AuthorText: post.Text,
				Label:      "no-labels",
			}

			if err := dsmt.db.Create(&item).Error; err != nil {
				return fmt.Errorf("failed to insert log: %w", err)
			}
			logger.Info("logged", "label", "no-labels")
		} else {
			logger.Info("no labels to emit or log")
		}
		return nil
	}

	for _, l := range labels {
		if isWatchedOp {
			if err := dsmt.emitLabel(ctx, uri, l); err != nil {
				return fmt.Errorf("failed to label post with %s: %w", l, err)
			}
			logger.Info("emitted label", "label", l)
		}

		_, isLoggedLabel := dsmt.loggedLabels[l]
		if dsmt.db != nil && isLoggedLabel {
			item := LogItem{
				ParentDid:  opDid,
				AuthorDid:  event.Did,
				ParentUri:  parentUri,
				AuthorUri:  uri,
				ParentText: parent.Text,
				AuthorText: post.Text,
				Label:      l,
			}

			if err := dsmt.db.Create(&item).Error; err != nil {
				return fmt.Errorf("failed to insert log: %w", err)
			}
			logger.Info("logged", "label", l)
		}
	}

	return nil
}

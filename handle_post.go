package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/jetstream/pkg/models"
	"github.com/haileyok/dontshowmethis/sets"
)

func uriFromEvent(evt *models.Event) string {
	return fmt.Sprintf("at://%s/%s/%s", evt.Did, evt.Commit.Collection, evt.Commit.RKey)
}

func isPolDomain(url *url.URL) bool {
	domain := url.Hostname()
	domain = strings.ToLower(domain)
	domain = strings.TrimPrefix(domain, "www.")

	for _, d := range sets.PolDomains {
		if strings.Contains(domain, d) {
			return true
		}
	}
	return false
}

func (dsmt *DontShowMeThis) handlePost(ctx context.Context, event *models.Event, post *bsky.FeedPost) error {
	if event == nil || event.Commit == nil {
		return nil
	}

	if post.Embed == nil && post.Reply == nil && len(post.Facets) == 0 {
		return nil
	}

	if post.Embed != nil && post.Embed.EmbedExternal != nil && post.Embed.EmbedExternal.External != nil {
		external := post.Embed.EmbedExternal.External

		u, err := url.Parse(external.Uri)
		if err != nil {
			return err
		}

		if isPolDomain(u) {
			if err := dsmt.emitLabel(ctx, uriFromEvent(event), LabelPolLink); err != nil {
				return err
			}
			return nil
		}
	}

	for _, f := range post.Facets {
		for _, ff := range f.Features {
			if ff.RichtextFacet_Link == nil {
				continue
			}

			u, err := url.Parse(ff.RichtextFacet_Link.Uri)
			if err != nil {
				return err
			}

			if isPolDomain(u) {
				if err := dsmt.emitLabel(ctx, uriFromEvent(event), LabelPolLink); err != nil {
					return err
				}
				return nil
			}
		}
	}

	// if post.Reply != nil {
	// 	ism, err := dsmt.r.SIsMember(ctx, RedisPrefix+LabelPolLink, post.Reply.Root.Uri).Result()
	// 	if err != nil && err != redis.Nil {
	// 		return err
	// 	}
	//
	// 	if ism {
	// 		if err := dsmt.emitLabel(ctx, uriFromEvent(event), LabelPolLinkReply); err != nil {
	// 			return err
	// 		}
	// 		return nil
	// 	}
	// }

	return nil
}

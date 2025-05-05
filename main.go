package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/util"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/bluesky-social/jetstream/pkg/client"
	"github.com/bluesky-social/jetstream/pkg/client/schedulers/sequential"
	"github.com/bluesky-social/jetstream/pkg/models"
	_ "github.com/joho/godotenv/autoload"
	"github.com/redis/go-redis/v9"
	"github.com/urfave/cli/v2"
)

const (
	RedisPrefix       = "dsmt/"
	LabelPolLink      = "pol-link"
	LabelPolLinkReply = "pol-link-reply"
)

func main() {
	app := &cli.App{
		Name:   "dontshowmethis",
		Action: run,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "jetstream-url",
				EnvVars: []string{"JETSTREAM_URL"},
				Value:   "wss://jetstream2.us-west.bsky.network/subscribe",
			},
			&cli.StringFlag{
				Name:     "labeler-url",
				Usage:    "skyware labeler event emission url",
				Required: true,
				EnvVars:  []string{"LABELER_URL"},
			},
			&cli.StringFlag{
				Name:     "labeler-key",
				Usage:    "skyware labeler event emission key",
				Required: true,
				EnvVars:  []string{"LABELER_KEY"},
			},
			&cli.StringFlag{
				Name:     "redis-addr",
				Usage:    "redis addr",
				Required: true,
				EnvVars:  []string{"REDIS_ADDR"},
			},
		},
	}

	app.Run(os.Args)
}

type DontShowMeThis struct {
	logger     *slog.Logger
	bskyClient *xrpc.Client
	h          *http.Client
	r          *redis.Client

	labelerUrl string
	labelerKey string
}

var run = func(cmd *cli.Context) error {
	dsmt := &DontShowMeThis{
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
		})),
		labelerUrl: cmd.String("labeler-url"),
		labelerKey: cmd.String("labeler-key"),
	}

	cli := &xrpc.Client{
		Host:    cmd.String("pds"),
		Headers: make(map[string]string),
		Auth:    &xrpc.AuthInfo{},
	}

	dsmt.bskyClient = cli

	dsmt.h = util.RobustHTTPClient()

	r := redis.NewClient(&redis.Options{
		Addr: cmd.String("redis-addr"),
	})

	dsmt.r = r

	dsmt.startConsumer(cmd.String("jetstream-url"))

	return nil
}

func (dsmt *DontShowMeThis) startConsumer(jsurl string) {
	config := client.DefaultClientConfig()
	config.WebsocketURL = jsurl
	config.Compress = true

	scheduler := sequential.NewScheduler("jetstream_localdev", dsmt.logger, dsmt.handleEvent)

	c, err := client.NewClient(config, dsmt.logger, scheduler)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	if err := c.ConnectAndRead(context.TODO(), nil); err != nil {
		log.Fatalf("failed to connect: %v", err)
	}

	dsmt.logger.Info("shutdown")
}

type handler struct {
	seenSeqs  map[int64]struct{}
	highwater int64
}

func (dsmt *DontShowMeThis) handleEvent(ctx context.Context, event *models.Event) error {
	if event.Commit != nil && (event.Commit.Operation == models.CommitOperationCreate || event.Commit.Operation == models.CommitOperationUpdate) {
		switch event.Commit.Collection {
		case "app.bsky.feed.post":
			var post bsky.FeedPost
			if err := json.Unmarshal(event.Commit.Record, &post); err != nil {
				return fmt.Errorf("failed to unmarshal post: %w", err)
			}

			if err := dsmt.handlePost(ctx, event, &post); err != nil {
				dsmt.logger.Error("error handling post", "error", err)
			}
		}
	}
	return nil
}

type EmitLabelRequest struct {
	Uri   string `json:"uri"`
	Label string `json:"label"`
}

func (dsmt *DontShowMeThis) emitLabel(ctx context.Context, uri, label string) error {
	body := &EmitLabelRequest{
		Uri:   uri,
		Label: label,
	}

	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", dsmt.labelerUrl+"/emit", bytes.NewReader(b))
	if err != nil {
		return err
	}

	req.Header.Set("authorization", "Bearer "+dsmt.labelerKey)
	req.Header.Set("content-type", "application/json")

	resp, err := dsmt.h.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received invalid status code from server: %d", resp.StatusCode)
	}

	if _, err := dsmt.r.SAdd(ctx, RedisPrefix+label, uri).Result(); err != nil {
		return err
	}

	return nil
}

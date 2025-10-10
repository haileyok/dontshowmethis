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
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/util"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/bluesky-social/jetstream/pkg/client"
	"github.com/bluesky-social/jetstream/pkg/client/schedulers/sequential"
	"github.com/bluesky-social/jetstream/pkg/models"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	_ "github.com/joho/godotenv/autoload"
	"github.com/urfave/cli/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	LabelBadFaith = "bad-faith"
	LabelOffTopic = "off-topic"
	LabelFunny    = "funny"
)

func main() {
	app := &cli.App{
		Name:   "dontshowmethis",
		Action: run,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "pds-url",
				EnvVars:  []string{"PDS_URL"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "account-handle",
				EnvVars:  []string{"ACCOUNT_HANDLE"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "account-password",
				EnvVars:  []string{"ACCOUNT_PASSWORD"},
				Required: true,
			},
			&cli.StringSliceFlag{
				Name:     "watched-ops",
				EnvVars:  []string{"WATCHED_OPS"},
				Required: true,
			},
			&cli.StringSliceFlag{
				Name:    "watched-log-ops",
				EnvVars: []string{"WATCHED_LOG_OPS"},
			},
			&cli.StringSliceFlag{
				Name:    "logged-labels",
				EnvVars: []string{"LOGGED_LABELS"},
			},
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
				Name:     "lmstudio-host",
				Usage:    "lmstudio host",
				EnvVars:  []string{"LMSTUDIO_HOST"},
				Required: true,
			},
			&cli.StringFlag{
				Name:    "log-db",
				Usage:   "name of the logging db (sqlite)",
				EnvVars: []string{"LOG_DB_NAME"},
			},
		},
	}

	app.Run(os.Args)
}

type DontShowMeThis struct {
	logger *slog.Logger
	xrpcc  *xrpc.Client
	httpc  *http.Client

	watchedOps    map[string]struct{}
	watchedLogOps map[string]struct{}
	loggedLabels  map[string]struct{}

	labelerUrl string
	labelerKey string

	lmstudioc *LMStudioClient

	postCache *lru.LRU[string, *bsky.FeedPost]

	db *gorm.DB
}

var run = func(cmd *cli.Context) error {
	opt := struct {
		PdsUrl          string
		JetstreamUrl    string
		AccountHandle   string
		AccountPassword string
		WatchedOps      []string
		WatchedLogOps   []string
		LoggedLabels    []string
		LabelerUrl      string
		LabelerKey      string
		LmstudioHost    string
		LogDbName       string
	}{
		PdsUrl:          cmd.String("pds-url"),
		JetstreamUrl:    cmd.String("jetstream-url"),
		AccountHandle:   cmd.String("account-handle"),
		AccountPassword: cmd.String("account-password"),
		WatchedOps:      cmd.StringSlice("watched-ops"),
		WatchedLogOps:   cmd.StringSlice("watched-log-ops"),
		LoggedLabels:    cmd.StringSlice("logged-labels"),
		LabelerUrl:      cmd.String("labeler-url"),
		LabelerKey:      cmd.String("labeler-key"),
		LmstudioHost:    cmd.String("lmstudio-host"),
		LogDbName:       cmd.String("log-db"),
	}

	if len(opt.LoggedLabels) > 0 && opt.LogDbName == "" {
		return fmt.Errorf("attempting to log labels, but did not include a db name in arguments")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	watchedOps := make(map[string]struct{}, len(opt.WatchedOps))
	for _, op := range opt.WatchedOps {
		watchedOps[op] = struct{}{}
	}

	watchedLogOps := make(map[string]struct{}, len(opt.WatchedLogOps))
	for _, op := range opt.WatchedLogOps {
		watchedLogOps[op] = struct{}{}
	}

	loggedLabels := make(map[string]struct{}, len(opt.LoggedLabels))
	for _, op := range opt.LoggedLabels {
		loggedLabels[op] = struct{}{}
	}

	xrpcc := &xrpc.Client{
		Host: opt.PdsUrl,
		// Headers: make(map[string]string),
		// Auth:    &xrpc.AuthInfo{},
	}

	httpc := util.RobustHTTPClient()

	lmstudioc := NewLMStudioClient(opt.LmstudioHost, logger)

	postCache := lru.NewLRU[string, *bsky.FeedPost](100, nil, 1*time.Hour)

	dsmt := &DontShowMeThis{
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
		})),
		labelerUrl:    opt.LabelerUrl,
		labelerKey:    opt.LabelerKey,
		watchedOps:    watchedOps,
		watchedLogOps: watchedLogOps,
		loggedLabels:  loggedLabels,
		xrpcc:         xrpcc,
		httpc:         httpc,
		lmstudioc:     lmstudioc,
		postCache:     postCache,
	}

	if opt.LogDbName != "" {
		db, err := gorm.Open(sqlite.Open(opt.LogDbName), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to create gorm db: %w", err)
		}
		logger.Info("opened gorm db for logging")
		dsmt.db = db
	}

	dsmt.startConsumer(cmd.String("jetstream-url"))

	return nil
}

func (dsmt *DontShowMeThis) startConsumer(jetstreamUrl string) {
	config := client.DefaultClientConfig()
	config.WebsocketURL = jetstreamUrl
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

	resp, err := dsmt.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received invalid status code from server: %d", resp.StatusCode)
	}

	return nil
}

func (dsmt *DontShowMeThis) getPost(ctx context.Context, uri string) (*bsky.FeedPost, error) {
	post, ok := dsmt.postCache.Get(uri)
	if ok {
		return post, nil
	}

	resp, err := bsky.FeedGetPosts(ctx, dsmt.xrpcc, []string{uri})
	if err != nil {
		return nil, fmt.Errorf("failed to get post: %w", err)
	}

	if resp == nil || len(resp.Posts) == 0 {
		return nil, fmt.Errorf("failed to get posts (empty response)")
	}

	postView := resp.Posts[0]
	post, ok = postView.Record.Val.(*bsky.FeedPost)
	if !ok {
		return nil, fmt.Errorf("failed to get post (invalid record)")
	}

	dsmt.postCache.Add(uri, post)

	return post, nil
}

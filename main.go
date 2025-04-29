package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/bluesky-social/jetstream/pkg/client"
	"github.com/bluesky-social/jetstream/pkg/client/schedulers/sequential"
	"github.com/bluesky-social/jetstream/pkg/models"
	_ "github.com/joho/godotenv/autoload"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:   "dontshowmethis",
		Action: run,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "ozone",
				EnvVars: []string{"OZONE"},
				// Required: true,
			},
			&cli.StringFlag{
				Name:    "pds",
				EnvVars: []string{"PDS"},
				Value:   "https://bsky.social",
				// Required: true,
			},
			&cli.StringFlag{
				Name:    "username",
				EnvVars: []string{"USERNAME"},
				// Required: true,
			},
			&cli.StringFlag{
				Name:    "password",
				EnvVars: []string{"PASSWORD"},
				// Required: true,
			},
			&cli.StringFlag{
				Name:    "jetstream-url",
				EnvVars: []string{"JETSTREAM_URL"},
				Value:   "wss://jetstream2.us-west.bsky.network/subscribe",
			},
		},
	}

	app.Run(os.Args)
}

type DontShowMeThis struct {
	logger *slog.Logger
	client *xrpc.Client
}

var run = func(cmd *cli.Context) error {
	dsmt := &DontShowMeThis{
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
		})),
	}

	dsmt.startConsumer(cmd.String("jetstream-url"))

	cli := &xrpc.Client{
		Host:    cmd.String("pds"),
		Headers: make(map[string]string),
		Auth:    &xrpc.AuthInfo{},
	}

	// if loaded, err := dsmt.loadSession(); !loaded {
	// 	if err != nil {
	// 		fmt.Printf("there was an error when loading the session: %v", err)
	// 	}
	//
	// 	res, err := atproto.ServerCreateSession(context.TODO(), cli, &atproto.ServerCreateSession_Input{
	// 		Identifier: cmd.String("username"),
	// 		Password:   cmd.String("password"),
	// 	})
	//
	// 	if err != nil {
	// 		return err
	// 	}
	//
	// 	cli.Auth.Did = res.Did
	// 	cli.Auth.Handle = res.Handle
	// 	cli.Auth.AccessJwt = res.AccessJwt
	// 	cli.Auth.RefreshJwt = res.RefreshJwt
	//
	// 	dsmt.saveSession()
	// }
	//
	// cli.Headers = map[string]string{
	// 	"atproto-proxy": cmd.String("ozone") + "#atproto_labeler",
	// }

	dsmt.client = cli

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

type session struct {
	Did        string `json:"did"`
	Handle     string `json:"handle"`
	AccessJwt  string `json:"access_jwt"`
	RefreshJwt string `json:"refresh_jwt"`
}

func (dsmt *DontShowMeThis) loadSession() (bool, error) {
	b, err := os.ReadFile("./auth.auth")
	if err != nil {
		return false, err
	}

	var s session
	err = json.Unmarshal(b, &s)
	if err != nil {
		return false, err
	}

	dsmt.client.Auth.Did = s.Did
	dsmt.client.Auth.Handle = s.Handle
	dsmt.client.Auth.AccessJwt = s.AccessJwt
	dsmt.client.Auth.RefreshJwt = s.RefreshJwt

	return true, nil
}

func (dsmt *DontShowMeThis) saveSession() {
	s := &session{
		Did:        dsmt.client.Auth.Did,
		Handle:     dsmt.client.Auth.Handle,
		AccessJwt:  dsmt.client.Auth.AccessJwt,
		RefreshJwt: dsmt.client.Auth.RefreshJwt,
	}

	// write to file
	b, err := json.Marshal(s)
	if err != nil {
		dsmt.logger.Error("error saving session", "err", err)
		return
	}

	err = os.WriteFile("./auth.auth", b, 0644)
	if err != nil {
		dsmt.logger.Error("error saving session", "err", err)
	}
}

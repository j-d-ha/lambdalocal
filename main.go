package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/urfave/cli/v3"
)

var line = strings.Repeat("-", 75) //nolint:gochecknoglobals,mnd

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, w io.Writer) error { //nolint:funlen,cyclop
	logLevel := slog.LevelInfo

	cmd := &cli.Command{
		Usage: "A tool for invoking AWS Lambdas locally",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "address",
				Aliases: []string{"a"},
				Value:   "localhost:8000",
				Usage:   "Address of locally running lambda. Port can be set with env var _LAMBDA_SERVER_PORT.",
			},
			&cli.BoolFlag{
				Name:    "parse-json",
				Aliases: []string{"p"},
				Value:   false,
				Usage:   "Parse response values like 'body' as JSON.",
			},
			&cli.IntFlag{
				Name:    "executionLimit",
				Aliases: []string{"e"},
				Value:   5, //nolint:mnd
				Usage:   "Execution time limit for this lambda in seconds.",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Enable verbose logging for debugging.",
				Action: func(_ context.Context, _ *cli.Command, b bool) error {
					if b {
						logLevel = slog.LevelDebug
					}

					return nil
				},
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "api",
				Usage: "Run local API and invoke lambda with requests",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "port",
						Aliases: []string{"p"},
						Value:   "8080",
						Usage:   "Port for local API Gateway. Must be a string of four digits .",
						Action: func(_ context.Context, _ *cli.Command, v string) error {
							re := regexp.MustCompile(`^\d{4}$`)
							if !re.MatchString(v) {
								return fmt.Errorf("expected 4 consecutive digets. Got %s", v)
							}

							return nil
						},
					},
					&cli.StringFlag{
						Name:    "template",
						Aliases: []string{"t"},
						Value:   "./template.yaml",
						Usage:   "Path to AWS SAM template.yaml.",
						Action: func(_ context.Context, _ *cli.Command, v string) error {
							_, err := os.Stat(v)
							if os.IsNotExist(err) {
								return fmt.Errorf("template '%s' does not exist", v)
							}

							return nil
						},
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					// get flags
					executionLimit := time.Duration(cmd.Int("executionLimit")) * time.Second
					lambdaAddress := cmd.String("address")
					port := cmd.String("port")
					template := cmd.String("template")
					parseJSON := cmd.Bool("parse-json")

					logger := slog.New(
						tint.NewHandler(
							w, &tint.Options{
								Level:      logLevel,
								TimeFormat: "15:04:05.000",
							},
						),
					)

					// create lambda client
					lambdaRPC := NewLambdaLambdaRPCClient(lambdaAddress, executionLimit)

					// run local API gateway
					if err := RunLambdaAPI(ctx, w, lambdaRPC, port, template, parseJSON, logger); err != nil {
						return fmt.Errorf("[in run.api] RunLambdaAPI failed: %w", err)
					}

					return nil
				},
			},
			{
				Name:  "event",
				Usage: "Invoke lambda with JSON event",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "file",
						Aliases: []string{"f"},
						Usage:   "Load event from `FILE_PATH`.",
						Action: func(_ context.Context, _ *cli.Command, v string) error {
							_, err := os.Stat(v)
							if os.IsNotExist(err) {
								return fmt.Errorf("event file '%s' does not exist", v)
							}

							return nil
						},
					},
					&cli.StringFlag{
						Name:    "string",
						Aliases: []string{"e"},
						Usage:   "Lambda event as a `STRING` to invoke.",
					},
				},
				Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
					filePath := cmd.String("file")
					event := cmd.String("string")

					// validate that both event and file-event not set
					if filePath != "" && event != "" {
						return ctx, errors.New("'file-event' and 'event' are mutually exclusive")
					}

					// if file path, read file to string
					if filePath != "" {
						file, err := os.Open(filePath) //nolint:gosec
						if err != nil {
							return ctx, fmt.Errorf(
								"[in run.event] failed to open event file: %w",
								err,
							)
						}
						defer func() {
							_ = file.Close()
						}()

						bytes, err := io.ReadAll(file)
						if err != nil {
							return ctx, fmt.Errorf(
								"[in run.event] failed to read event file: %w",
								err,
							)
						}

						if err = cmd.Set("string", string(bytes)); err != nil {
							return ctx, fmt.Errorf(
								"[in run.event] failed to set value for key 'string': %w",
								err,
							)
						}

						return ctx, nil
					}

					return ctx, nil
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					// get flags
					executionLimit := time.Duration(cmd.Int("executionLimit")) * time.Second
					lambdaAddress := cmd.String("address")
					event := cmd.String("string")
					parseJSON := cmd.Bool("parse-json")

					logger := slog.New(
						tint.NewHandler(
							w, &tint.Options{
								Level:      logLevel,
								TimeFormat: "15:04:05.000",
							},
						),
					)

					// create lambda client
					lambdaRPC := NewLambdaLambdaRPCClient(lambdaAddress, executionLimit)

					// invoke lambda with event
					if err := RunLambdaEvent(ctx, w, lambdaRPC, event, parseJSON, logger); err != nil {
						return fmt.Errorf("[in run.event] RunLambdaEvent failed: %w", err)
					}

					return nil
				},
			},
		},
	}

	if err := cmd.Run(ctx, os.Args); err != nil {
		return fmt.Errorf("[in run] Run failed: %w", err)
	}

	return nil
}

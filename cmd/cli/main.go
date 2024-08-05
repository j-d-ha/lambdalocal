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
	"time"

	"github.com/j-d-ha/lambdalocal"
	"github.com/urfave/cli/v3"
)

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	logger := slog.Default()

	cmd := &cli.Command{
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
		},
		Commands: []*cli.Command{
			{
				Name:  "api",
				Usage: "run lambda as an api",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "port",
						Aliases: []string{"p"},
						Value:   "8080",
						Usage:   "Port for local API Gateway. Must be a string of four digits .",
						Action: func(ctx context.Context, cmd *cli.Command, v string) error {
							re := regexp.MustCompile(`^\d{4}$`)
							if re.MatchString(v) {
								return fmt.Errorf("expected 4 consecutive digets. Got %v", v)
							}
							return nil
						},
					},
					&cli.StringFlag{
						Name:    "template",
						Aliases: []string{"t"},
						Value:   "./template.yaml",
						Usage:   "Path to AWS SAM template.yaml.",
						Action: func(ctx context.Context, cmd *cli.Command, v string) error {
							_, err := os.Stat(v)
							if os.IsNotExist(err) {
								return errors.New(fmt.Sprintf("template '%v' does not exist", v))
							}

							return nil
						},
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					// get flags
					executionLimit := time.Duration(cmd.Int("executionLimit")) * time.Second
					lambdaAddress := cmd.String("lambdaAddress")
					port := cmd.String("port")
					template := cmd.String("template")
					parseJSON := cmd.Bool("parse-json")

					lambdaRPC := lambdalocal.NewLambdaRPC(lambdaAddress, executionLimit)

					// run local API gateway
					if err := lambdalocal.RunLambdaAPI(ctx, lambdaRPC, port, template, parseJSON, logger); err != nil {
						return fmt.Errorf("[in main.run.api] RunLambdaAPI failed: %w", err)
					}

					return nil
				},
			},
			{
				Name:  "event",
				Usage: "invoke lambda with event",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "file",
						Aliases: []string{"f"},
						Usage:   "Load event from `FILE_PATH`.",
						Action: func(ctx context.Context, cmd *cli.Command, v string) error {
							_, err := os.Stat(v)
							if os.IsNotExist(err) {
								return errors.New(fmt.Sprintf("event file '%v' does not exist", v))
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
				Before: func(ctx context.Context, cmd *cli.Command) error {
					filePath := cmd.String("file")
					event := cmd.String("string")

					// validate that both event and file-event not set
					if filePath != "" && event != "" {
						return errors.New("'file-event' and 'event' are mutually exclusive")
					}

					// if file path, read file to string
					if filePath != "" {
						file, err := os.Open(filePath)
						if err != nil {
							return fmt.Errorf("[in main.run.event] failed to open event file: %w", err)
						}
						defer func() {
							if err = file.Close(); err != nil {
								logger.Error("Error closing file", "err", err)
							}
						}()

						bytes, err := io.ReadAll(file)
						if err != nil {
							return fmt.Errorf("[in main.run.event] failed to read event file: %w", err)
						}

						if err = cmd.Set("string", string(bytes)); err != nil {
							return fmt.Errorf("[in main.run.event] failed to set value for key 'string': %w", err)
						}

						return nil
					}

					return nil
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					// get flags
					executionLimit := time.Duration(cmd.Int("executionLimit")) * time.Second
					lambdaAddress := cmd.String("lambdaAddress")
					event := cmd.String("string")
					parseJSON := cmd.Bool("parse-json")

					lambdaRPC := lambdalocal.NewLambdaRPC(lambdaAddress, executionLimit)

					// invoke lambda with event
					if err := lambdalocal.RunLambdaEvent(ctx, lambdaRPC, event, parseJSON, logger); err != nil {
						return fmt.Errorf("[in main.run.event] RunLambdaEvent failed: %w", err)
					}

					return nil
				},
			},
		},
	}

	if err := cmd.Run(ctx, os.Args); err != nil {
		return fmt.Errorf("[in main.run] Run failed: %w", err)
	}

	return nil
}

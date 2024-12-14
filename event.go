package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

func RunLambdaEvent(
	_ context.Context,
	w io.Writer,
	lambdaRPC lambdaCaller,
	event string,
	parseJSON bool,
	logger *slog.Logger,
) error {
	_, _ = fmt.Fprintln(w, line)

	logger.Info("Starting local Lambda invocation with Event")

	logger.Debug("Invoking lambda event")

	invokeResponse, err := lambdaRPC.Invoke([]byte(event))
	if err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaEvent] invoke failed: %w", err)
	}

	if err = printResponse(logger, invokeResponse, parseJSON); err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaEvent] printResponse failed: %w", err)
	}

	logger.Info("Lambda invocation complete, Exiting...")

	_, _ = fmt.Fprintln(w, line)

	return nil
}

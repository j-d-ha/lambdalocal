package lambdalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

func RunLambdaEvent(ctx context.Context, lambdaRPC lambdaCaller, event string, parseJSON bool, logger *slog.Logger) error { //nolint:lll
	println(line) //nolint:forbidigo
	logger.Info("Starting local Lambda invocation with Event")

	invokeResponse, err := lambdaRPC.Invoke([]byte(event))
	if err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaEvent] invoke failed: %w", err)
	}

	if invokeResponse.Error != nil {
		logger.Error("Lambda returned error", "invokeResponse.Error", invokeResponse.Error)
	}

	response := make(map[string]any)
	if err = json.Unmarshal(invokeResponse.Payload, &response); err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaEvent] unmarshal response failed: %w", err)
	}

	if parseJSON {
		response = parseInnerJSON(response)
	}

	out, err := json.MarshalIndent(response, "", "    ")
	if err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaEvent] MarshalIndent response failed: %w", err)
	}

	logger.Info("Lambda returned")
	fmt.Println(string(out)) //nolint:forbidigo

	return nil
}

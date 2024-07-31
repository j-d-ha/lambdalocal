package lambdalocal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

func RunLambdaEvent(ctx context.Context, address, event string, parseJSON bool, executionLimit time.Duration, logger *slog.Logger) error {
	println(line)
	logger.Info("Starting local Lambda invocation with Event")

	invokeResponse, err := invoke(address, []byte(event), executionLimit)
	if err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaEvent] invoke failed: %w", err)
	}

	if invokeResponse.Error != nil {
		logger.Error("Lambda returned error", "invokeResponse.Error", invokeResponse.Error)
		return nil
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
	fmt.Println(string(out))

	return nil
}

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aws/aws-lambda-go/lambda/messages"
)

func printResponse(
	logger *slog.Logger,
	invokeResponse messages.InvokeResponse,
	parseJSON bool,
) error {
	logger.Debug("Handling lambda event response")

	if invokeResponse.Error != nil {
		var builder strings.Builder

		builder.WriteString(fmt.Sprintf("Returned error: %s\n", invokeResponse.Error.Message))
		builder.WriteString("\nStack trace:\n")

		for _, detail := range invokeResponse.Error.StackTrace {
			builder.WriteString(fmt.Sprintf("%s:%d - %s\n", detail.Path, detail.Line, detail.Label))
		}

		logger.Error("Lambda returned error:\n" + builder.String())
	}

	if invokeResponse.Payload == nil {
		logger.Debug("Lambda returned no payload")
		return nil
	}

	response := make(map[string]any)
	if err := json.Unmarshal(invokeResponse.Payload, &response); err != nil {
		logger.Info("Lambda returned non-JSON payload:\n" + string(invokeResponse.Payload))
		return nil //nolint:nilerr
	}

	if parseJSON {
		response = parseInnerJSON(response)
	}

	out, err := json.MarshalIndent(response, "", "    ")
	if err != nil {
		return fmt.Errorf("[in lambdalocal.printResponse] MarshalIndent response failed: %w", err)
	}

	logger.Info("Lambda returned JSON payload:\n" + string(out))

	return nil
}

// parseInnerJSON walks all key value pairs on response and attempt to unmarshal
// strings to JSON.
func parseInnerJSON(data map[string]any) map[string]any {
	for k, v := range data {
		if vv, ok := v.(string); ok {
			newJSON := any(nil)
			if err := json.Unmarshal([]byte(vv), &newJSON); err != nil {
				continue
			}

			data[k] = newJSON
		}
	}

	return data
}

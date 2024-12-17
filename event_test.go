package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/lambda/messages"
	"github.com/lithammer/dedent"
	"github.com/lmittmann/tint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLambdaCaller is a mock implementation of the lambdaCaller interface
type MockLambdaCaller struct {
	mock.Mock
}

func (m *MockLambdaCaller) Invoke(data []byte) (messages.InvokeResponse, error) {
	args := m.Called(data)
	return args.Get(0).(messages.InvokeResponse), args.Error(1) //nolint:wrapcheck,forcetypeassert
}

func TestRunLambdaEvent(t *testing.T) { //nolint:funlen
	tests := map[string]struct {
		event          string
		parseJSON      bool
		invokeResp     messages.InvokeResponse
		invokeErr      error
		expectedOutput string
		expectedErr    error
	}{
		"successful invocation without JSON parsing": {
			event:     `{"key": "value"}`,
			parseJSON: false,
			invokeResp: messages.InvokeResponse{
				Payload: []byte(`{"response": "success"}`),
			},
			invokeErr: nil,
			expectedOutput: `
				---------------------------------------------------------------------------
				0 INF Starting local Lambda invocation with Event
				0 DBG Invoking lambda event
				0 DBG Handling lambda event response
				0 INF Lambda returned JSON payload:
				{
				    "response": "success"
				}
				0 INF Lambda invocation complete, Exiting...
				---------------------------------------------------------------------------
			`,
			expectedErr: nil,
		},
		"successful invocation with JSON parsing": {
			event:     `{"key": "value"}`,
			parseJSON: true,
			invokeResp: messages.InvokeResponse{
				Payload: []byte(`{"response": "{\"innerKey\": \"innerValue\"}"}`),
			},
			invokeErr: nil,
			expectedOutput: `
				---------------------------------------------------------------------------
				0 INF Starting local Lambda invocation with Event
				0 DBG Invoking lambda event
				0 DBG Handling lambda event response
				0 INF Lambda returned JSON payload:
				{
				    "response": {
				        "innerKey": "innerValue"
				    }
				}
				0 INF Lambda invocation complete, Exiting...
				---------------------------------------------------------------------------
			`,
			expectedErr: nil,
		},
		"invoke error": {
			event:      `{"key": "value"}`,
			parseJSON:  false,
			invokeResp: messages.InvokeResponse{},
			invokeErr:  errors.New("invoke error"),
			expectedOutput: `
				---------------------------------------------------------------------------
				0 INF Starting local Lambda invocation with Event
				0 DBG Invoking lambda event
			`,
			expectedErr: fmt.Errorf("[in lambdalocal.RunLambdaEvent] invoke failed: %w", errors.New("invoke error")),
		},
		"lambda returned error - valid JSON body": {
			event:     `{"key": "value"}`,
			parseJSON: false,
			invokeResp: messages.InvokeResponse{
				Payload: []byte(`{"error": "test error"}`),
				Error: &messages.InvokeResponse_Error{
					Message: "test error",
					Type:    "",
					StackTrace: []*messages.InvokeResponse_Error_StackFrame{
						{
							Path:  "path1",
							Line:  1,
							Label: "part_1",
						},
						{
							Path:  "path2",
							Line:  2,
							Label: "part_2",
						},
					},
					ShouldExit: false,
				},
			},
			invokeErr: nil,
			expectedOutput: `
				---------------------------------------------------------------------------
				0 INF Starting local Lambda invocation with Event
				0 DBG Invoking lambda event
				0 DBG Handling lambda event response
				0 ERR Lambda returned error:
				Returned error: test error
				
				Stack trace:
				path1:1 - part_1
				path2:2 - part_2
				
				0 INF Lambda returned JSON payload:
				{
				    "error": "test error"
				}
				0 INF Lambda invocation complete, Exiting...
				---------------------------------------------------------------------------
			`,
			expectedErr: nil,
		},
		"lambda returned error - invalid JSON body": {
			event:     `{"key": "value"}`,
			parseJSON: false,
			invokeResp: messages.InvokeResponse{
				Payload: []byte(`invalid JSON`),
				Error: &messages.InvokeResponse_Error{
					Message: "test error",
					Type:    "",
					StackTrace: []*messages.InvokeResponse_Error_StackFrame{
						{
							Path:  "path1",
							Line:  1,
							Label: "part_1",
						},
						{
							Path:  "path2",
							Line:  2,
							Label: "part_2",
						},
					},
					ShouldExit: false,
				},
			},
			invokeErr: nil,
			expectedOutput: `
				---------------------------------------------------------------------------
				0 INF Starting local Lambda invocation with Event
				0 DBG Invoking lambda event
				0 DBG Handling lambda event response
				0 ERR Lambda returned error:
				Returned error: test error
				
				Stack trace:
				path1:1 - part_1
				path2:2 - part_2
				
				0 INF Lambda returned non-JSON payload:
				invalid JSON
				0 INF Lambda invocation complete, Exiting...
				---------------------------------------------------------------------------
			`,
			expectedErr: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockLambdaRPC := new(MockLambdaCaller)

			var buf bytes.Buffer

			logger := slog.New(
				tint.NewHandler(
					&buf, &tint.Options{
						Level:      slog.LevelDebug,
						TimeFormat: "0",
						NoColor:    true,
					},
				),
			)

			mockLambdaRPC.On("Invoke", []byte(tc.event)).Return(tc.invokeResp, tc.invokeErr)

			err := RunLambdaEvent(context.Background(), &buf, mockLambdaRPC, tc.event, tc.parseJSON, logger)

			if tc.expectedErr == nil {
				require.NoError(t, err)
			} else {
				assert.Equal(t, tc.expectedErr, err)
			}

			output := strings.TrimSpace(buf.String())

			// fmt.Println(output)

			assert.Equal(t, strings.TrimSpace(dedent.Dedent(tc.expectedOutput)), output)

			mockLambdaRPC.AssertExpectations(t)
		})
	}
}

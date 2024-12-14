package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/lambda/messages"
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

func TestRunLambdaEvent(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		event       string
		parseJSON   bool
		invokeResp  messages.InvokeResponse
		invokeErr   error
		expectedErr string
	}{
		"successful invocation without JSON parsing": {
			event:     `{"key": "value"}`,
			parseJSON: false,
			invokeResp: messages.InvokeResponse{
				Payload: []byte(`{"response": "success"}`),
			},
			invokeErr:   nil,
			expectedErr: "",
		},
		"successful invocation with JSON parsing": {
			event:     `{"key": "value"}`,
			parseJSON: true,
			invokeResp: messages.InvokeResponse{
				Payload: []byte(`{"response": "{\"innerKey\": \"innerValue\"}"}`),
			},
			invokeErr:   nil,
			expectedErr: "",
		},
		"invoke error": {
			event:       `{"key": "value"}`,
			parseJSON:   false,
			invokeResp:  messages.InvokeResponse{},
			invokeErr:   errors.New("invoke error"),
			expectedErr: "invoke failed: invoke error",
		},
		"unmarshal error": {
			event:     `{"key": "value"}`,
			parseJSON: false,
			invokeResp: messages.InvokeResponse{
				Payload: []byte(`invalid json`),
			},
			invokeErr:   nil,
			expectedErr: "unmarshal response failed: invalid character 'i' looking for beginning of value",
		},
	}

	for name, tc := range tests {
		t.Run(
			name, func(t *testing.T) {
				t.Parallel()

				mockLambdaRPC := new(MockLambdaCaller)
				logger := slog.Default()

				mockLambdaRPC.On("Invoke", []byte(tc.event)).Return(tc.invokeResp, tc.invokeErr)

				err := RunLambdaEvent(context.Background(), os.Stdout, mockLambdaRPC, tc.event, tc.parseJSON, logger)

				if tc.expectedErr == "" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tc.expectedErr)
				}

				mockLambdaRPC.AssertExpectations(t)
			},
		)
	}
}

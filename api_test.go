package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-lambda-go/lambda/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGatewayHandler(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		route              apiRoute
		parseJSON          bool
		requestPath        string
		requestMethod      string
		expectedStatus     int
		expectedResponse   string
		mockInvokeResponse messages.InvokeResponse
		mockInvokeError    error
	}{
		"Successful invocation": {
			route:            apiRoute{path: "/test1", method: http.MethodGet},
			parseJSON:        true,
			requestPath:      "/test1",
			requestMethod:    http.MethodGet,
			expectedStatus:   http.StatusOK,
			expectedResponse: `{"message":"success"}`,
			mockInvokeResponse: messages.InvokeResponse{
				Payload: []byte(`{"body":"{\"message\":\"success\"}","StatusCode":200}`),
				Error:   nil,
			},
			mockInvokeError: nil,
		},
		"Invocation failure": {
			route:              apiRoute{path: "/test2", method: http.MethodGet},
			parseJSON:          true,
			requestPath:        "/test2",
			requestMethod:      http.MethodGet,
			expectedStatus:     http.StatusServiceUnavailable,
			expectedResponse:   "Service Unavailable\n",
			mockInvokeResponse: messages.InvokeResponse{},
			mockInvokeError:    errors.New("invoke error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mockLambdaRPC := new(MockLambdaCaller)
			logger := slog.Default()

			mockLambdaRPC.
				On("Invoke", mock.Anything).
				Return(tc.mockInvokeResponse, tc.mockInvokeError).
				Once()

			req := httptest.NewRequest(tc.requestMethod, tc.requestPath, nil)
			rr := httptest.NewRecorder()

			handler := gatewayHandler(mockLambdaRPC, tc.parseJSON, tc.route, logger)
			handler.ServeHTTP(rr, req)

			resp := rr.Result()
			body, _ := io.ReadAll(resp.Body)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			assert.Equal(t, tc.expectedResponse, string(body))

			mockLambdaRPC.AssertExpectations(t)
		})
	}
}

func TestParseHTTPRequest(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		method        string
		target        string
		body          string
		headers       map[string]string
		pathParamKeys []string
		resourcePath  string
		expectedEvent genericAPIEvent
		expectError   bool
	}{
		"GET request with query params": {
			method:        http.MethodGet,
			target:        "/test?id=123",
			body:          "",
			headers:       map[string]string{"Content-Type": "application/json"},
			pathParamKeys: []string{"id"},
			resourcePath:  "/test",
			expectedEvent: genericAPIEvent{
				Resource:                        "/test",
				Path:                            "/test",
				HTTPMethod:                      http.MethodGet,
				Headers:                         map[string]string{"Content-Type": "application/json"},
				MultiValueHeaders:               map[string][]string{"Content-Type": {"application/json"}},
				QueryStringParameters:           map[string]string{"id": "123"},
				MultiValueQueryStringParameters: map[string][]string{"id": {"123"}},
				PathParameters:                  map[string]string{},
				Body:                            "",
			},
			expectError: false,
		},
		"POST request with body and headers": {
			method:        http.MethodPost,
			target:        "/test",
			body:          `{"name": "test"}`,
			headers:       map[string]string{"Content-Type": "application/json", "Authorization": "Bearer token"},
			pathParamKeys: []string{},
			resourcePath:  "/test",
			expectedEvent: genericAPIEvent{
				Resource:                        "/test",
				Path:                            "/test",
				HTTPMethod:                      http.MethodPost,
				Headers:                         map[string]string{"Content-Type": "application/json", "Authorization": "Bearer token"}, //nolint:lll
				MultiValueHeaders:               map[string][]string{"Content-Type": {"application/json"}, "Authorization": {"Bearer token"}},
				QueryStringParameters:           map[string]string{},
				MultiValueQueryStringParameters: map[string][]string{},
				PathParameters:                  map[string]string{},
				Body:                            `{"name": "test"}`,
			},
			expectError: false,
		},
		"GET request with multi-value headers and query params": {
			method:        http.MethodGet,
			target:        "/test?id=123&name=test&name=example",
			body:          "",
			headers:       map[string]string{"Content-Type": "application/json", "Accept": "application/json;application/xml"},
			pathParamKeys: []string{"id"},
			resourcePath:  "/test",
			expectedEvent: genericAPIEvent{
				Resource:   "/test",
				Path:       "/test",
				HTTPMethod: http.MethodGet,
				Headers: map[string]string{
					"Content-Type": "application/json",
					"Accept":       "application/json;application/xml",
				},
				MultiValueHeaders: map[string][]string{
					"Content-Type": {"application/json"},
					"Accept":       {"application/json;application/xml"},
				},
				QueryStringParameters: map[string]string{
					"id":   "123",
					"name": "example",
				},
				MultiValueQueryStringParameters: map[string][]string{
					"id":   {"123"},
					"name": {"test", "example"},
				},
				PathParameters: map[string]string{},
				Body:           "",
			},
			expectError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, tc.target, bytes.NewReader([]byte(tc.body)))
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			eventByte, err := parseHTTPRequest(req, tc.pathParamKeys, tc.resourcePath)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				var actualEvent genericAPIEvent
				err = json.Unmarshal(eventByte, &actualEvent)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedEvent, actualEvent)
			}
		})
	}
}

func TestOutputLambdaResponse(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input          messages.InvokeResponse
		parseJSON      bool
		expectedOutput string
		expectedError  bool
	}{
		"successful parse": {
			input: messages.InvokeResponse{
				Payload: []byte(`{"body": "{\"message\": \"Hello World\"}", "other": [1, 2, 3]}`),
				Error: &messages.InvokeResponse_Error{
					Message: "this is a test error",
					Type:    "test error",
					StackTrace: []*messages.InvokeResponse_Error_StackFrame{{
						Path:  "test path",
						Line:  5,
						Label: "test label",
					}},
				},
			},
			parseJSON: true,
			expectedOutput: `
			{
				"Error": {
					"errorMessage": "this is a test error",
					"errorType": "test error",
					"stackTrace": [
						{
							"path": "test path",
							"line": 5,
							"label": "test label"
						}
					]
				},
				"Payload": {
					"body": {
						"message": "Hello World"
					},
					"other": [
						1,
						2,
						3
					]
				}
			}`,
			expectedError: false,
		},
		"failed unmarshal": {
			input: messages.InvokeResponse{
				Payload: []byte(`{"body": "{\"message\": \"Hello World\"}, "other": [1, 2, 3]`),
				Error: &messages.InvokeResponse_Error{
					Message: "this is a test error",
					Type:    "test error",
					StackTrace: []*messages.InvokeResponse_Error_StackFrame{{
						Path:  "test path",
						Line:  5,
						Label: "test label",
					}},
				},
			},
			parseJSON:      true,
			expectedOutput: "",
			expectedError:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			output, err := outputLambdaResponse(tc.input, tc.parseJSON)
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.JSONEq(t, tc.expectedOutput, output)
			}
		})
	}
}

func TestReturnHTTPResponse(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		invokeResponse  messages.InvokeResponse
		expectedStatus  int
		expectedHeaders map[string]string
		expectedBody    string
		expectError     bool
	}{
		"successful response": {
			invokeResponse: messages.InvokeResponse{
				Payload: []byte(`{
					"statusCode": 200,
					"headers": {"Content-Type": "application/json"},
					"body": "{\"message\": \"Hello World\"}"
				}`),
			},
			expectedStatus: http.StatusOK,
			expectedHeaders: map[string]string{
				"Content-Type": "application/json",
			},
			expectedBody: `{"message": "Hello World"}`,
			expectError:  false,
		},
		"unmarshal error": {
			invokeResponse: messages.InvokeResponse{
				Payload: []byte(`{`),
			},
			expectedStatus:  http.StatusInternalServerError,
			expectedHeaders: map[string]string{},
			expectedBody:    "",
			expectError:     true,
		},
		"missing status code": {
			invokeResponse: messages.InvokeResponse{
				Payload: []byte(`{
					"headers": {"Content-Type": "application/json"},
					"body": "{\"message\": \"Hello World\"}"
				}`),
			},
			expectedStatus: http.StatusInternalServerError,
			expectedHeaders: map[string]string{
				"Content-Type": "application/json",
			},
			expectedBody: `{"message": "Hello World"}`,
			expectError:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			err := returnHTTPResponse(rec, tc.invokeResponse)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				result := rec.Result()
				defer func() {
					if err := result.Body.Close(); err != nil {
						require.NoError(t, err)
					}
				}()
				assert.Equal(t, tc.expectedStatus, result.StatusCode)

				for k, v := range tc.expectedHeaders {
					assert.Equal(t, v, result.Header.Get(k))
				}

				body, _ := io.ReadAll(result.Body)
				assert.JSONEq(t, tc.expectedBody, string(body))
			}
		})
	}
}

type mockOSFileReader struct {
	mock.Mock
}

func (m *mockOSFileReader) read(filename string) ([]byte, error) {
	args := m.Called(filename)

	return args.Get(0).([]byte), args.Error(1)
}

func TestParseTemplate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mockInput      []any
		mockReturn     []any
		yamlContent    string
		expectedRoutes []apiRoute
		expectedErrStr string
	}{
		"valid template with one route": {
			mockInput: []any{""},
			mockReturn: []any{
				[]byte(`
Resources:
  MyLambdaFunction:
    Type: "AWS::Serverless::Function"
    Properties:
      Events:
        ApiEvent:
          Type: "Api"
          Properties:
            Path: "/my/path"
            Method: "get"
                `),
				nil,
			},
			expectedRoutes: []apiRoute{
				{method: "GET", path: "/my/path"},
			},
			expectedErrStr: "",
		},
		"valid template with two routes": {
			mockInput: []any{""},
			mockReturn: []any{
				[]byte(`
Resources:
  FirstLambdaFunction:
    Type: "AWS::Serverless::Function"
    Properties:
      Events:
        FirstApiEvent:
          Type: "Api"
          Properties:
            Path: "/first/path"
            Method: "post"
  SecondLambdaFunction:
    Type: "AWS::Serverless::Function"
    Properties:
      Events:
        SecondApiEvent:
          Type: "Api"
          Properties:
            Path: "/second/path"
            Method: "put"
                `),
				nil,
			},
			expectedRoutes: []apiRoute{
				{method: "POST", path: "/first/path"},
				{method: "PUT", path: "/second/path"},
			},
			expectedErrStr: "",
		},
		"valid template with no routes": {
			mockInput: []any{""},
			mockReturn: []any{
				[]byte(`
Resources:
  HelloWorldFunction:
    Type: AWS::Serverless::Function
    Metadata:
      BuildMethod: go1.x
                `),
				nil,
			},
			expectedRoutes: []apiRoute(nil),
			expectedErrStr: "",
		},
		"file read error": {
			mockInput:      []any{""},
			mockReturn:     []any{[]byte{}, errors.New("test error")},
			yamlContent:    "",
			expectedRoutes: []apiRoute{},
			expectedErrStr: "[in lambdalocal.parseTemplate] read file failed:",
		},
		"unmarshal error": {
			mockInput: []any{""},
			mockReturn: []any{
				[]byte(`
Resources:
  MyLambdaFunction:
    Type: "AWS::Serverless::Function"
    Properties:
      Events:
        ApiEvent:
          Type: "Api"
          Properties:
            Path: ["/my/path"]
            Method: "get"
                `),
				nil,
			},
			expectedRoutes: []apiRoute{},
			expectedErrStr: "[in lambdalocal.parseTemplate] unmarshal yaml failed:",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mockReader := new(mockOSFileReader)
			mockReader.
				On("read", tc.mockInput...).
				Return(tc.mockReturn...).
				Once()

			result, err := parseTemplate("", mockReader)

			assert.Equal(t, tc.expectedRoutes, result)
			if tc.expectedErrStr != "" {
				assert.ErrorContains(t, err, tc.expectedErrStr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

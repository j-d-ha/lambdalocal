package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-lambda-go/lambda/messages"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

const shutdownDuration = 5 * time.Second

type apiRoute struct {
	method string
	path   string
}

type lambdaCaller interface {
	Invoke(data []byte) (messages.InvokeResponse, error)
}

// RunLambdaAPI initializes and starts a local API Gateway to invoke AWS Lambda
// functions. ctx is the context that controls the server's lifecycle. w is the
// writer where server outputs or logs will be written. lambdaRPC is the
// interface used to call Lambda functions. port is the port on which the local
// API Gateway will run. templatePath specifies the location of the API Gateway
// template file. parseJSON indicates if the request and response data should be
// JSON-parsed. logger is used to log the information and errors during
// execution. Returns an error if setup or execution fails.
func RunLambdaAPI(
	ctx context.Context,
	w io.Writer,
	lambdaRPC lambdaCaller,
	port, templatePath string,
	parseJSON bool,
	logger *slog.Logger,
) error {
	_, _ = fmt.Fprintln(w, line)

	logger.Info("Starting local API Gateway for Lambda")

	routes, err := parseTemplate(templatePath, osFileReader{})
	if err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaAPI] parseTemplate failed: %w", err)
	}

	if len(routes) == 0 {
		return errors.New("[in lambdalocal.RunLambdaAPI] no routes found")
	}

	if err = runServer(ctx, w, lambdaRPC, port, routes, parseJSON, logger); err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaAPI] runServer failed: %w", err)
	}

	return nil
}

func runServer(
	ctx context.Context,
	w io.Writer,
	lambdaRPC lambdaCaller,
	port string,
	routes []apiRoute,
	parseJSON bool,
	logger *slog.Logger,
) error {
	addr := fmt.Sprintf("%s:%s", "localhost", port)
	router := http.NewServeMux()

	// register routes from template.yaml
	for _, route := range routes {
		logger.Info(fmt.Sprintf("%s http://%s%s", route.method, addr, route.path))
		router.Handle(
			fmt.Sprintf("%s %s", route.method, route.path),
			gatewayHandler(lambdaRPC, parseJSON, route, logger),
		)
	}

	// Create a simple HTTP server
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second, //nolint:mnd
	}

	wg, ctx := errgroup.WithContext(ctx)

	// Channel to listen for interrupt or termination signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	wg.Go(
		func() error {
			<-quit

			_, _ = fmt.Fprintln(w, line)

			// Create a deadline to wait for
			ctx, cancel := context.WithTimeout(ctx, shutdownDuration)
			defer cancel()

			// Attempt a graceful shutdown
			logger.Info("Shutting down server...")

			if err := server.Shutdown(ctx); err != nil {
				return fmt.Errorf("[in lambdalocal.RunLambdaAPI] Server forced to shutdown: %w", err)
			}

			logger.Info("Server Shut down")

			return nil
		},
	)

	// Start the server in a separate goroutine
	logger.Info("Starting server on " + addr)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("[in lambdalocal.RunLambdaAPI] ListenAndServe: %w", err)
	}

	if err := wg.Wait(); err != nil {
		return err //nolint:wrapcheck
	}

	logger.Info("Server Shut down, exiting...")

	return nil
}

type genericAPIEvent struct {
	Resource                        string              `json:"resource"`
	Path                            string              `json:"path"`
	HTTPMethod                      string              `json:"httpMethod"`
	Headers                         map[string]string   `json:"headers"`
	MultiValueHeaders               map[string][]string `json:"multiValueHeaders"`
	QueryStringParameters           map[string]string   `json:"queryStringParameters"`
	MultiValueQueryStringParameters map[string][]string `json:"multiValueQueryStringParameters"`
	PathParameters                  map[string]string   `json:"pathParameters"`
	Body                            string              `json:"body"`
}

type genericAPIResponse struct {
	StatusCode        int                 `json:"statusCode"`
	Headers           map[string]string   `json:"headers"`
	MultiValueHeaders map[string][]string `json:"multiValueHeaders"`
	Body              string              `json:"body"`
	IsBase64Encoded   bool                `json:"isBase64Encoded,omitempty"`
}

func gatewayHandler(
	lambdaRPC lambdaCaller,
	parseJSON bool,
	route apiRoute,
	logger *slog.Logger,
) http.Handler {
	// get path param keys
	re := regexp.MustCompile(`{([^}]*)}`)
	matches := re.FindAllStringSubmatch(route.path, -1)

	var pathParamKeys []string

	for _, match := range matches {
		if len(match) > 1 {
			pathParamKeys = append(pathParamKeys, match[1])
		}
	}

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Println(line) //nolint:forbidigo
			logger.Info("Handling request for: " + route.path)
			logger.Info("URL request path: " + r.URL.Path)

			eventByte, err := parseHTTPRequest(r, pathParamKeys, route.path)
			if err != nil {
				logger.Error("[in lambdalocal.RunLambdaAPI] parseHTTPRequest failed", "err", err)
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusBadRequest)

				return
			}

			invokeResponse, err := lambdaRPC.Invoke(eventByte)
			if err != nil {
				logger.Error("[in lambdalocal.RunLambdaAPI] invoke failed", "err", err)
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

				return
			}

			if err = printResponse(logger, invokeResponse, parseJSON); err != nil {
				logger.Error("[in lambdalocal.RunLambdaAPI] printResponse failed", "err", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

				return
			}

			if err = returnHTTPResponse(w, invokeResponse); err != nil {
				logger.Error("[in lambdalocal.RunLambdaAPI] returnHTTPResponse failed", "err", err)
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusInternalServerError)

				return
			}
		},
	)
}

func parseHTTPRequest(r *http.Request, pathParamKeys []string, resourcePath string) ([]byte, error) {
	// read body
	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("[in lambdalocal.RunLambdaAPI] failed to read request body: %w", err)
	}

	// path parameters
	pathParams := make(map[string]string)

	for _, paramKey := range pathParamKeys {
		paramValue := r.PathValue(paramKey)
		if paramValue != "" {
			pathParams[paramKey] = paramValue
		}
	}

	// Extract headers
	headers := make(map[string]string)
	multiValueHeaders := make(map[string][]string)

	for key, values := range r.Header {
		headers[key] = values[0]
		multiValueHeaders[key] = values
	}

	// Extract query string parameters
	queryStringParameters := make(map[string]string)
	multiValueQueryStringParameters := make(map[string][]string)

	for key, values := range r.URL.Query() {
		queryStringParameters[key] = values[len(values)-1]
		multiValueQueryStringParameters[key] = values
	}

	eventByte, err := json.Marshal(
		genericAPIEvent{
			Resource:                        resourcePath,
			Path:                            r.URL.Path,
			HTTPMethod:                      r.Method,
			Headers:                         headers,
			MultiValueHeaders:               multiValueHeaders,
			QueryStringParameters:           queryStringParameters,
			MultiValueQueryStringParameters: multiValueQueryStringParameters,
			PathParameters:                  pathParams,
			Body:                            string(requestBody),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("[in lambdalocal.parseHTTPRequest] marshal response failed: %w", err)
	}

	return eventByte, nil
}

func outputLambdaResponse(invokeResponse messages.InvokeResponse, parseJSON bool) (string, error) {
	responseMap := make(map[string]any)

	responseMap["Error"] = invokeResponse.Error

	responseBody := make(map[string]any)
	if err := json.Unmarshal(invokeResponse.Payload, &responseBody); err != nil {
		return "", fmt.Errorf("[in lambdalocal.outputLambdaResponse] unmarshal response failed: %w", err)
	}

	if parseJSON {
		responseBody = parseInnerJSON(responseBody)
	}

	responseMap["Payload"] = responseBody

	out, err := json.MarshalIndent(responseMap, "", "    ")
	if err != nil {
		return "", fmt.Errorf("[in lambdalocal.outputLambdaResponse] marshal response failed: %w", err)
	}

	return string(out), nil
}

func returnHTTPResponse(w http.ResponseWriter, invokeResponse messages.InvokeResponse) error {
	APIResponse := genericAPIResponse{}

	if invokeResponse.Error != nil {
		http.Error(w, invokeResponse.Error.Message, http.StatusInternalServerError)

		return nil //nolint:nilerr
	}

	if err := json.Unmarshal(invokeResponse.Payload, &APIResponse); err != nil {
		return fmt.Errorf("[in lambdalocal.returnHTTPResponse] Unmarshal payload failed: %w", err)
	}

	// headers
	for k, v := range APIResponse.Headers {
		w.Header().Set(k, v)
	}

	// status code
	if APIResponse.StatusCode == 0 {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(APIResponse.StatusCode)
	}

	// body
	if _, err := w.Write([]byte(APIResponse.Body)); err != nil {
		return fmt.Errorf("[in lambdalocal.returnHTTPResponse] Write body failed: %w", err)
	}

	return nil
}

type samTemplate struct {
	Resources map[string]struct {
		Type       string `yaml:"Type"` //nolint:tagliatelle
		Properties struct {
			Events map[string]struct {
				Type       string `yaml:"Type"` //nolint:tagliatelle
				Properties struct {
					Path   string `yaml:"Path"`   //nolint:tagliatelle
					Method string `yaml:"Method"` //nolint:tagliatelle
				} `yaml:"Properties"` //nolint:tagliatelle
			} `yaml:"Events"` //nolint:tagliatelle
		} `yaml:"Properties"` //nolint:tagliatelle
	} `yaml:"Resources"` //nolint:tagliatelle
}

type osFileReader struct{}

func (osFileReader) read(name string) ([]byte, error) {
	return os.ReadFile(name) //nolint:gosec,wrapcheck
}

type fileReader interface {
	read(name string) ([]byte, error)
}

func parseTemplate(templatePath string, reader fileReader) ([]apiRoute, error) {
	yamlFile, err := reader.read(templatePath)
	if err != nil {
		return []apiRoute{}, fmt.Errorf("[in lambdalocal.parseTemplate] read file failed: %w", err)
	}

	SAMData := samTemplate{}
	if err = yaml.Unmarshal(yamlFile, &SAMData); err != nil {
		return []apiRoute{}, fmt.Errorf("[in lambdalocal.parseTemplate] unmarshal yaml failed: %w", err)
	}

	var routes []apiRoute

	for _, resource := range SAMData.Resources {
		for _, event := range resource.Properties.Events {
			routes = append(
				routes, apiRoute{
					method: strings.ToUpper(event.Properties.Method),
					path:   event.Properties.Path,
				},
			)
		}
	}

	return routes, nil
}

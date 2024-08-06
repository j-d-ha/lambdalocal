package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-lambda-go/lambda/messages"
	"gopkg.in/yaml.v3"
)

const shutdownDuration = 5 * time.Second

var line = strings.Repeat("-", 75) //nolint:gochecknoglobals

type apiRoute struct {
	method string
	path   string
}

type LambdaAPI struct {
	logger *slog.Logger
}

type lambdaCaller interface {
	Invoke(data []byte) (messages.InvokeResponse, error)
}

func RunLambdaAPI(ctx context.Context, lambdaRPC lambdaCaller, port, templatePath string, parseJSON bool, logger *slog.Logger) error { //nolint:lll
	fmt.Println(line)
	logger.Info("Starting local API Gateway for Lambda")

	routes, err := parseTemplate(templatePath, osFileReader{})
	if err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaAPI] parseTemplate failed: %w", err)
	}
	if len(routes) == 0 {
		return errors.New("[in lambdalocal.RunLambdaAPI] no routes found")
	}

	if err = runServer(ctx, lambdaRPC, port, routes, parseJSON, logger); err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaAPI] runServer failed: %w", err)
	}

	return nil
}

func runServer(ctx context.Context, lambdaRPC lambdaCaller, port string, routes []apiRoute, parseJSON bool, logger *slog.Logger) (err error) { //nolint:lll
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
	server := &http.Server{ //nolint:gosec // will not be used in production so not a problem
		Addr:    addr,
		Handler: router,
	}

	// Channel to listen for interrupt or termination signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	errChan := make(chan error, 1)

	// Start the server in a separate goroutine
	go func() {
		log.Println("Starting server on " + addr)
		if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("[in lambdalocal.RunLambdaAPI] ListenAndServe: %w", err)
		}
	}()

	select {
	// Wait for the interrupt signal
	case <-quit:
		fmt.Println(line) //nolint:forbidigo

		// Create a deadline to wait for
		ctx, cancel := context.WithTimeout(ctx, shutdownDuration)
		defer cancel()

		// Attempt a graceful shutdown
		logger.Info("Shutting down server...")
		if err = server.Shutdown(ctx); err != nil {
			return fmt.Errorf("[in lambdalocal.RunLambdaAPI] Server forced to shutdown: %w", err)
		}

		logger.Info("Server Shut down")

		return nil

	case err = <-errChan:
		return err
	}
}

type genericAPIEvent struct {
	Resource                        string              `json:"resource"` // The resource path defined for the gatewayHandler
	Path                            string              `json:"path"`     // The url path for the caller
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

func gatewayHandler(lambdaRPC lambdaCaller, parseJSON bool, route apiRoute, logger *slog.Logger) http.Handler { //nolint:funlen

	// get path param keys
	re := regexp.MustCompile(`\{([^}]*)\}`)
	matches := re.FindAllStringSubmatch(route.path, -1)

	var pathParamKeys []string
	for _, match := range matches {
		if len(match) > 1 {
			pathParamKeys = append(pathParamKeys, match[1])
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		formattedResponse, err := outputLambdaResponse(invokeResponse, parseJSON)
		if err != nil {
			logger.Error("[in lambdalocal.RunLambdaAPI] invoke failed", "err", err)
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

			return
		}
		logger.Info("Lambda response:")
		fmt.Println(formattedResponse)

		if err = returnHTTPResponse(w, invokeResponse); err != nil {
			logger.Error("[in lambdalocal.RunLambdaAPI] returnHTTPResponse failed", "err", err)
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusInternalServerError)

			return
		}
	})
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

	eventByte, err := json.Marshal(genericAPIEvent{
		Resource:                        resourcePath,
		Path:                            r.URL.Path,
		HTTPMethod:                      r.Method,
		Headers:                         headers,
		MultiValueHeaders:               multiValueHeaders,
		QueryStringParameters:           queryStringParameters,
		MultiValueQueryStringParameters: multiValueQueryStringParameters,
		PathParameters:                  pathParams,
		Body:                            string(requestBody),
	})
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
		Type       string `yaml:"Type"`
		Properties struct {
			Events map[string]struct {
				Type       string `yaml:"Type"`
				Properties struct {
					Path   string `yaml:"Path"`
					Method string `yaml:"Method"`
				} `yaml:"Properties"`
			} `yaml:"Events"`
		} `yaml:"Properties"`
	} `yaml:"Resources"`
}

type osFileReader struct{}

func (osFileReader) read(name string) ([]byte, error) {
	return os.ReadFile(name)
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
			routes = append(routes, apiRoute{
				method: strings.ToUpper(event.Properties.Method),
				path:   event.Properties.Path,
			})
		}
	}

	return routes, nil
}

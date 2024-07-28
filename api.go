package lambdalocal

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

	"github.com/aws/aws-lambda-go/events"
	"gopkg.in/yaml.v3"
)

var line = strings.Repeat("-", 75)

type apiRoute struct {
	method string
	path   string
}

func RunLambdaAPI(ctx context.Context, lambdaAddr, port, templatePath string, parseJSON bool, logger *slog.Logger) error {
	println(line)
	logger.Info("Starting local API Gateway for Lambda")

	routes, err := parseTemplate(templatePath)
	if err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaAPI] parseTemplate failed: %w", err)
	}
	if err = runServer(ctx, lambdaAddr, port, routes, parseJSON, logger); err != nil {
		return fmt.Errorf("[in lambdalocal.RunLambdaAPI] runServer failed: %w", err)
	}

	return nil
}

func runServer(ctx context.Context, lambdaAddr, port string, routes []apiRoute, parseJSON bool, logger *slog.Logger) (err error) {
	addr := fmt.Sprintf("%s:%s", "localhost", port)
	router := http.NewServeMux()
	for _, route := range routes {
		logger.Info(route.method + " http://" + addr + route.path)
		router.Handle(route.method+" "+route.path, handler(lambdaAddr, parseJSON, route, logger))
	}

	// Create a simple HTTP server
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Channel to listen for interrupt or termination signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Start the server in a separate goroutine
	go func() {
		log.Println("Starting server on " + addr)
		if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = fmt.Errorf("[in lambdalocal.RunLambdaAPI] ListenAndServe: %w", err)
		}
	}()

	// Wait for the interrupt signal
	<-quit
	fmt.Println(line)

	// Create a deadline to wait for
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Attempt a graceful shutdown
	logger.Info("Shutting down server...")
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server Shut down")

	return nil
}

func handler(lambdaAddr string, parseJSON bool, route apiRoute, logger *slog.Logger) http.Handler {
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
		fmt.Println(line)
		logger.Info("Handling request for: " + route.path)
		logger.Info("URL request path: " + r.URL.Path)

		// read body
		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("[in lambdalocal.RunLambdaAPI] read request body failed", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
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
			headers[key] = strings.Join(values, ",")
			multiValueHeaders[key] = values
		}

		// Extract query string parameters
		queryStringParameters := make(map[string]string)
		multiValueQueryStringParameters := make(map[string][]string)
		for key, values := range r.URL.Query() {
			queryStringParameters[key] = strings.Join(values, ",")
			multiValueQueryStringParameters[key] = values
		}

		eventByte, err := json.Marshal(events.APIGatewayProxyRequest{
			Resource:                        route.path,
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
			logger.Error("[in lambdalocal.RunLambdaAPI] marshal event failed", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		invokeResponse, err := invoke(lambdaAddr, eventByte, 5*time.Second)
		if err != nil {
			logger.Error("[in lambdalocal.RunLambdaAPI] invoke failed", "err", err)
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}

		if invokeResponse.Error != nil {
			logger.Error("Lambda returned error")
			errorBytes, _ := json.MarshalIndent(invokeResponse.Error, "", "    ")
			fmt.Println(string(errorBytes))
		}

		response := make(map[string]any)
		if err = json.Unmarshal(invokeResponse.Payload, &response); err != nil {
			logger.Error("[in lambdalocal.RunLambdaEvent] unmarshal response failed", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		if parseJSON {
			response = parseInnerJSON(response)
		}

		out, err := json.MarshalIndent(response, "", "    ")
		if err != nil {
			logger.Error("[in lambdalocal.RunLambdaEvent] MarshalIndent response failed", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		logger.Info("Lambda return:")
		fmt.Println(string(out))

		APIGatewayResponse := events.APIGatewayProxyResponse{}
		if err := json.Unmarshal(invokeResponse.Payload, &APIGatewayResponse); err != nil {
			logger.Error("[in lambdalocal.RunLambdaAPI] Unmarshal payload failed", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		// headers
		for k, v := range APIGatewayResponse.Headers {
			w.Header().Set(k, v)
		}

		// status code
		if APIGatewayResponse.StatusCode == 0 {
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.WriteHeader(APIGatewayResponse.StatusCode)

		// body
		if _, err := w.Write([]byte(APIGatewayResponse.Body)); err != nil {
			logger.Error("[in lambdalocal.RunLambdaAPI] failed to encode response body", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	})
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

func parseTemplate(templatePath string) ([]apiRoute, error) {
	yamlFile, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("[in lambdalocal.parseTemplate] read file failed: %w", err)
	}

	SAMdata := samTemplate{}
	if err = yaml.Unmarshal(yamlFile, &SAMdata); err != nil {
		return nil, fmt.Errorf("[in lambdalocal.parseTemplate] unmarshal yaml failed: %w", err)
	}

	var routes []apiRoute

	for _, resource := range SAMdata.Resources {
		for _, event := range resource.Properties.Events {
			routes = append(routes, apiRoute{
				method: event.Properties.Method,
				path:   event.Properties.Path,
			})
		}
	}

	return routes, nil
}

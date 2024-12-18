# lambdalocal

[![Go Report Card](https://goreportcard.com/badge/github.com/j-d-ha/lambdalocal)](https://goreportcard.com/report/github.com/j-d-ha/lambdalocal)

CLI tool for invoking AWS Lambdas locally

## Overview

`lambdalocal` is a Go based commandline tool for invoking locally running AWS Lambda functions for
development, testing, and debugging.

It has two modes:

- `api` and `event`. `api` starts a local API server and parses incoming requests into events and
  then invokes those events against a locally running lambda using RPC. The data returned from the
  lambda is printed out and then returned to as an API response to the original request
    - `api` has been tested with Go based API Gateway Lambdas and ALB Target Group Lambdas.

- `event` takes an input AWS Lambda event JSON, either from a file or a string, and invokes a
  locally running lambda with that event using RPC. The data returned from the lambda is printed
  out.

## Installation

Install latest version with

```bash
go install github.com/j-d-ha/lambdalocal@latest
```

## Usage

`lambdalocal -h`

```text
NAME:
   lambdalocal - A tool for invoking AWS Lambdas locally

USAGE:
   lambdalocal [global options] [command [command options]] [arguments...]

COMMANDS:
   api      Run local API and invoke lambda with requests
   event    Invoke lambda with JSON event
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --address value, -a value         Address of locally running lambda. Port can be set with env var _LAMBDA_SERVER_PORT. (default: "localhost:8000")
   --parse-json, -p                  Parse response values like 'body' as JSON. (default: false)
   --executionLimit value, -e value  Execution time limit for this lambda in seconds. (default: 5)
   --verbose, -v                     Enable verbose logging for debugging. (default: false)
   --help, -h                        show help (default: false)
```

`lambdalocal api -h`

```text
NAME:
   lambdalocal api - Run local API and invoke lambda with requests

USAGE:
   lambdalocal api [command [command options]] 

OPTIONS:
   --port value, -p value      Port for local API Gateway. Must be a string of four digits . (default: "8080")
   --template value, -t value  Path to AWS SAM template.yaml. (default: "./template.yaml")
   --help, -h                  show help (default: false)
```

`lambdalocal event -h`

```text
NAME:
   lambdalocal event - Invoke lambda with JSON event

USAGE:
   lambdalocal event [command [command options]] 

OPTIONS:
   --file FILE_PATH, -f FILE_PATH  Load event from FILE_PATH.
   --string STRING, -e STRING      Lambda event as a STRING to invoke.
   --help, -h                      show help (default: false)
```

## Examples

The following code examples demonstrate how to use `lambdalocal`:

- [Event invocation example](./example/event/)

## Issues and Support

If you find a bug or have an idea to improve the tool, please open an issue and I will do my best to
address it.

## Acknowledgements

This project builds off of blmayer's
project [awslambdarpc](https://github.com/blmayer/awslambdarpc).

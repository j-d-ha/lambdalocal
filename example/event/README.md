# lambdalocal Example: Invoke Event

## Introduction

## Walkthrough

For this example, we wil be using a basic Go Lambda that accepts a custom event and returns a custom
event. Do to the incredibly flexible nature of Lambdas, these events could be swapped out for any
other without materially changing the demonstration.

Lets start by looking at the Handler that is in `main.go`.

```go
package main

func main() {
	fmt.Println("Starting main()")

	lambda.Start(Handler)
}

type InputEvent struct {
	Name string `json:"name"`
}

type OutputEvent struct {
	Message string `json:"message"`
}

func Handler(event InputEvent) (OutputEvent, error) {
	fmt.Printf("Handler received event: %#v\n", event)

	if event.Name == "" {
		return OutputEvent{Message: "Hello, World!"}, nil
	}

	return OutputEvent{Message: fmt.Sprintf("Hello, %s!", event.Name)}, nil
}
```

Next, lets look at our event in `event.json`:

```json
{
  "name": "Gopher"
}
```

Now, lets start our lambda running locally:

```bash
export _LAMBDA_SERVER_PORT=8000 && go run main.go
```

With that running, lets invoke our lambda though an event using `lambdalocal`.

```bash
lambdalocal event --file event.json
```

Now we can see the output in the terminal:

```
---------------------------------------------------------------------------
21:37:31.471 INF Starting local Lambda invocation with Event
21:37:31.473 INF Lambda returned JSON payload:
{
    "message": "Hello, Gopher!"
}
21:37:31.473 INF Lambda invocation complete, Exiting...
---------------------------------------------------------------------------
```
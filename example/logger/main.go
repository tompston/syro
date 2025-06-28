package main

import (
	"github.com/tompston/syro"
)

func main() {
	logger := syro.NewConsoleLogger(nil).
		WithSource("my-event").
		WithEvent("my-source")

	logger.Info("Hello World")
}

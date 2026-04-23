//go:build windows

package main

import (
	"flag"

	"turing-display-go/internal/app"
)

func main() {
	flag.Parse()
	app.Run()
}

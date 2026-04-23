//go:build windows

package main

import (
	"flag"

	"turing-display-go/internal/displayapp"
)

func main() {
	flag.Parse()
	displayapp.Run()
}

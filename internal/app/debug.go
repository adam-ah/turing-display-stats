package app

import "flag"

var debugEnabled bool

func init() {
	flag.BoolVar(&debugEnabled, "debug", false, "enable console output")
}

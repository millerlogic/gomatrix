package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/GeertJohan/gomatrix"
	"github.com/jessevdk/go-flags"
)

func main() {
	// parse flags
	var opts gomatrix.Opts
	args, err := flags.Parse(&opts)
	if err != nil {
		flagError := err.(*flags.Error)
		if flagError.Type == flags.ErrHelp {
			return
		}
		if flagError.Type == flags.ErrUnknownFlag {
			fmt.Println("Use --help to view all available options.")
			return
		}
		fmt.Printf("Error parsing flags: %s\n", err)
		return
	}
	if len(args) > 0 {
		// we don't accept too much arguments..
		fmt.Printf("Unknown argument '%s'.\n", args[0])
		return
	}

	// seed the rand package with time
	rand.Seed(time.Now().UnixNano())

	os.Exit(gomatrix.Run(opts, nil, os.Stdout))
}

package main

import (
	"os"

	"github.com/mailgun/log"
	"github.com/mailgun/vulcand/plugin/registry"
	"github.com/mailgun/vulcand/vctl/command"
)

var vulcanUrl string

func main() {
	log.InitWithConfig(log.Config{Name: "console"})

	cmd := command.NewCommand(registry.GetRegistry())
	err := cmd.Run(os.Args)
	if err != nil {
		log.Errorf("error: %s\n", err)
	}
}

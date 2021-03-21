package main

import (
	"flag"
	"fmt"
	"github.com/womat/debug"
	"os"
	"relayhat/pkg/app"
	"relayhat/pkg/app/config"
)

const defaultConfigFile = "/opt/womat/conf/" + app.MODULE + ".yaml"

func main() {
	debug.SetDebug(os.Stderr, debug.Standard)
	cfg := config.NewConfig()

	flag.BoolVar(&cfg.Flag.Version, "version", false, "print version and exit")
	flag.StringVar(&cfg.Flag.Debug, "debug", "", "enable debug information (standard | trace | debug)")
	flag.StringVar(&cfg.Flag.ConfigFile, "config", defaultConfigFile, "config file")
	flag.Parse()

	if cfg.Flag.Version {
		fmt.Println(app.Version())
		os.Exit(1)
	}

	for {
		if err := cfg.LoadConfig(); err != nil {
			fmt.Print(err)
			os.Exit(1)
		}

		debug.SetDebug(cfg.Debug.File, cfg.Debug.Flag)

		a := app.New(cfg)

		debug.InfoLog.Printf("starting app %s", app.Version())

		if err := a.Run(); err != nil {
			debug.FatalLog.Print(err)
			os.Exit(1)
		}

		select {
		case <-a.Restart():
			debug.InfoLog.Println("restarting application")
			_ = cfg.Debug.File.Close()
		case <-a.Shutdown():
			debug.InfoLog.Println("shutdown application")
			_ = cfg.Debug.File.Close()
			return
		}
	}
}

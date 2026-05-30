package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"kratos-demo/internal/conf"
	"kratos-demo/internal/service"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
)

var (
	Name     = "kratos-demo"
	Version  = "0.1.0"
	flagconf string
	flagCLI  bool
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	flag.BoolVar(&flagCLI, "cli", false, "interactive local CLI (starts all agents, router-first)")
}

func main() {
	flag.Parse()

	if flagCLI {
		// Kratos config watcher logs to the global logger on Close(); keep CLI stdout clean.
		log.SetLogger(log.NewStdLogger(io.Discard))
	}

	c := config.New(
		config.WithSource(
			file.NewSource(flagconf),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	if flagCLI {
		sessionID := fmt.Sprintf("cli-%d", time.Now().Unix())
		processLog, err := service.NewCLIProcessLog(sessionID)
		if err != nil {
			panic(err)
		}
		defer processLog.Close()

		logger := log.With(log.NewStdLogger(processLog),
			"ts", log.DefaultTimestamp,
			"caller", log.DefaultCaller,
			"service.name", Name,
			"service.version", Version,
		)

		cli, cleanup, err := wireCLI(bc.GetData(), bc.GetAi(), bc.GetRuntime(), logger)
		if err != nil {
			panic(err)
		}
		defer cleanup()
		cli.BindSession(sessionID, processLog)
		cli.BindAIConfig(bc.GetAi())
		if err := cli.Run(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "cli error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.name", Name,
		"service.version", Version,
	)

	app, cleanup, err := wireApp(bc.GetServer(), bc.GetData(), bc.GetAi(), bc.GetRuntime(), logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		panic(err)
	}
}

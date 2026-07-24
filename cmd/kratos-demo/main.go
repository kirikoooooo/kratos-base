package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kratos-demo/internal/conf"
	"kratos-demo/internal/service"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
)

var (
	Name           = "kratos-demo"
	Version        = "0.1.0"
	flagconf       string
	flagCLI        bool
	flagCLISSEAddr string
)

func init() {
	flag.StringVar(&flagconf, "conf", "", "config directory or file; default: configs/ next to executable")
	flag.BoolVar(&flagCLI, "cli", false, "interactive local CLI (starts all agents, router-first)")
	flag.StringVar(&flagCLISSEAddr, "cli-sse-addr", "", "optional loopback address for CLI output SSE, e.g. 127.0.0.1:8010")
}

func main() {
	flag.Parse()

	if flagCLI {
		// Kratos config watcher logs to the global logger on Close(); keep CLI stdout clean.
		log.SetLogger(log.NewStdLogger(io.Discard))
	}

	confPath, err := conf.EnsureConfigPath(flagconf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	c := config.New(
		config.WithSource(
			file.NewSource(confPath),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "config load error: %v\n", err)
		os.Exit(1)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		fmt.Fprintf(os.Stderr, "config parse error: %v\n", err)
		os.Exit(1)
	}
	if info, err := os.Stat(confPath); err == nil {
		configDir := confPath
		if !info.IsDir() {
			configDir = filepath.Dir(confPath)
		}
		conf.ResolveFilePaths(configDir, bc.GetSecurity())
	}

	if flagCLI {
		printLangfuseCLIStatus(bc.GetObservability())
		// The runtime is constructed before CLIService can prompt for credentials.
		// Inject persisted credentials first so its lazy provider sees the active config.
		if creds, err := service.LoadCLICredentials(); err == nil {
			service.ApplyCLICredentials(bc.GetAi(), creds)
		}
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

		cli, cleanup, err := wireCLI(bc.GetData(), bc.GetAi(), bc.GetRuntime(), bc.GetObservability(), bc.GetSecurity(), logger)
		if err != nil {
			panic(err)
		}
		defer cleanup()
		cli.BindSession(sessionID, processLog)
		cli.BindAIConfig(bc.GetAi())
		if flagCLISSEAddr != "" {
			shutdownSSE, err := service.StartCLISSEServer(context.Background(), flagCLISSEAddr, cli.OutputStream())
			if err != nil {
				fmt.Fprintf(os.Stderr, "cli SSE error: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = shutdownSSE(context.Background()) }()
			fmt.Fprintf(os.Stderr, "CLI SSE: http://%s/debug/cli/stream?session_id=%s\n", flagCLISSEAddr, sessionID)
		}
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

	// 加载 .myagent/credentials.json 中的 API 凭证注入到 aiConf
	// （dashboard 模式下 config.yaml 的 api_key 为空，凭证通过 CLI /config 命令设置后持久化在此）
	if creds, err := service.LoadCLICredentials(); err == nil {
		service.ApplyCLICredentials(bc.GetAi(), creds)
	}

	app, cleanup, err := wireApp(bc.GetServer(), bc.GetData(), bc.GetAi(), bc.GetRuntime(), bc.GetObservability(), bc.GetSecurity(), logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		panic(err)
	}
}

func printLangfuseCLIStatus(config *conf.Observability) {
	cfg := config.GetLangfuse()
	if !cfg.GetEnabled() {
		fmt.Fprintln(os.Stderr, "Langfuse: disabled by configuration")
		return
	}
	publicKeyName := cfg.GetPublicKeyEnv()
	if publicKeyName == "" {
		publicKeyName = "LANGFUSE_PUBLIC_KEY"
	}
	secretKeyName := cfg.GetSecretKeyEnv()
	if secretKeyName == "" {
		secretKeyName = "LANGFUSE_SECRET_KEY"
	}
	_, publicKeySet := os.LookupEnv(publicKeyName)
	_, secretKeySet := os.LookupEnv(secretKeyName)
	if !publicKeySet || !secretKeySet || os.Getenv(publicKeyName) == "" || os.Getenv(secretKeyName) == "" {
		fmt.Fprintf(os.Stderr, "Langfuse: disabled; missing environment variable(s): %s=%t %s=%t\n", publicKeyName, publicKeySet, secretKeyName, secretKeySet)
		return
	}
	host := os.Getenv("LANGFUSE_BASE_URL")
	if host == "" {
		host = cfg.GetHost()
	}
	fmt.Fprintf(os.Stderr, "Langfuse: enabled; OTLP endpoint=%s/api/public/otel/v1/traces\n", strings.TrimRight(host, "/"))
}

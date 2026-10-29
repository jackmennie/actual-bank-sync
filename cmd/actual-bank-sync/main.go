package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackmennie/actual-bank-sync/internal/actual"
	"github.com/jackmennie/actual-bank-sync/internal/bank"
	upbank "github.com/jackmennie/actual-bank-sync/internal/bank/up"
	"github.com/jackmennie/actual-bank-sync/internal/config"
	"github.com/jackmennie/actual-bank-sync/internal/setup"
	"github.com/jackmennie/actual-bank-sync/internal/syncer"
)

func main() {
	configPath, args, err := parseGlobalArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(args) == 0 {
		fmt.Println("Usage: actual-bank-sync <command>")
		fmt.Println()
		fmt.Println("Global options:")
		fmt.Println("  --config <path>  Configuration file (default: config.yaml)")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  setup    Configure account mappings")
		fmt.Println("  sync [--dry-run] [--verbose] [--report <csv>]  Import UP transactions into Actual")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch args[0] {
	case "setup":
		if err := runSetup(ctx, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "sync":
		dryRun, verbose, reportPath, err := parseSyncArgs(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		result, err := runSync(ctx, cfg, dryRun, verbose || cfg.Verbose)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if reportPath != "" {
			if err := syncer.WritePreviewCSVFile(reportPath, result.PreviewRows); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing preview report: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Preview report written: %s (%d changed transaction(s))\n", reportPath, len(result.PreviewRows))
		}
		if result.DryRun {
			fmt.Printf("Dry run complete: %d account(s) checked, %d transaction(s) would be added, %d updated; no transactions or checkpoints were written\n", result.Accounts, result.WouldAdd, result.WouldUpdate)
		} else {
			fmt.Printf("Sync complete: %d account(s), %d transaction(s) added, %d updated\n", result.Accounts, result.Imported, result.Updated)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		os.Exit(1)
	}
}

func parseGlobalArgs(args []string) (string, []string, error) {
	configPath := "config.yaml"
	commandArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] != "--config" {
			commandArgs = append(commandArgs, args[i])
			continue
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return "", nil, fmt.Errorf("--config requires a file path")
		}
		i++
		configPath = args[i]
	}
	return configPath, commandArgs, nil
}

func runSetup(ctx context.Context, cfg *config.Config) error {
	actualClient, banks := clients(cfg, cfg.Verbose)

	setupClient := setup.New(
		actualClient,
		banks,
		cfg.MappingsFile,
		os.Stdin,
		os.Stdout,
	)

	return setupClient.Run(ctx)
}

func runSync(ctx context.Context, cfg *config.Config, dryRun, verbose bool) (syncer.Result, error) {
	actualClient, banks := clients(cfg, verbose)
	job := syncer.New(
		actualClient,
		banks,
		cfg.MappingsFile,
		cfg.SyncStateFile,
		cfg.InitialLookbackDays,
	)
	job.SetVerbose(verbose)
	return job.Run(ctx, dryRun)
}

func parseSyncArgs(args []string) (dryRun bool, verbose bool, reportPath string, err error) {
	dryRun = false
	verbose = false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--verbose":
			verbose = true
		case "--report":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return false, false, "", fmt.Errorf("--report requires a CSV path")
			}
			i++
			reportPath = args[i]
		default:
			return false, false, "", fmt.Errorf("unknown sync option %q (supported: --dry-run, --verbose, --report <csv>)", arg)
		}
	}
	if reportPath != "" && !dryRun {
		return false, false, "", fmt.Errorf("--report can only be used with --dry-run")
	}
	return dryRun, verbose, reportPath, nil
}

func clients(cfg *config.Config, verbose bool) (*actual.Client, map[string]bank.BankClient) {
	actualClient := actual.NewClient(actual.ClientConfig{
		BaseURL:            cfg.Actual.BaseURL,
		Password:           cfg.Actual.Password,
		SyncID:             cfg.Actual.SyncID,
		EncryptionPassword: cfg.Actual.EncryptionKey,
		DataDir:            cfg.Actual.DataDir,
		NodeBinary:         cfg.Actual.NodeBinary,
		HelperPath:         cfg.Actual.HelperPath,
		Verbose:            verbose || cfg.Verbose,
	})
	banks := map[string]bank.BankClient{
		"UP": upbank.NewClient(cfg.Providers.UP.APIKey),
	}
	return actualClient, banks
}

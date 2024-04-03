// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"log/slog"

	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/internal/services/inputreader"
	"github.com/ethereum/go-ethereum/common"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

const (
	LOCALHOST_URL  = "ws://localhost:8545"
	CMD_NAME       = "input-reader"
	RETRY_INTERVAL = 5
)

const (
	MODE_POLLING = iota
	MODE_BLOCK_SUBSCRIPTION
)

var Cmd = &cobra.Command{
	Use:   CMD_NAME,
	Short: "Reads inputs from RPC endpoint for a limited amount of time",
	Run:   run,
}

var (
	timeout    int
	verboseLog bool
)

func init() {
	Cmd.Flags().IntVarP(&timeout,
		"timeout",
		"t",
		60,
		"timeout in seconds")
	Cmd.Flags().BoolVarP(&verboseLog,
		"verbose",
		"v",
		false,
		"enable verbose logging")
}

func main() {
	opts := &tint.Options{
		Level: slog.LevelInfo,
	}

	if verboseLog {
		opts.Level = slog.LevelDebug
	}

	handler := tint.NewHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	err := Cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	slog.Info(CMD_NAME, "timeout", timeout)

	//	mostRecentFinalizedBlock := new(BlockRef)
	//	mostRecentFinalizedBlock.Set(big.NewInt(20))

	///////////////////////////////////
	// Copied from node cmd for testing purposes
	ctx, stop := signal.NotifyContext(ctx /*context.Background()*/, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config := config.FromEnv()

	// setup log
	opts := &tint.Options{
		Level:      config.LogLevel,
		AddSource:  config.LogLevel == slog.LevelDebug,
		NoColor:    !config.LogPretty || !isatty.IsTerminal(os.Stdout.Fd()),
		TimeFormat: "2006-01-02T15:04:05.000", // RFC3339 with milliseconds and without timezone
	}
	handler := tint.NewHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	///////////////////////////////////

	inputReader := newInputReader(config)

	ready := make(chan struct{}, 1)
	go inputReader.Start(ctx, ready)
Loop:
	for {
		select {
		case <-ready:
			slog.Info(CMD_NAME, "status", "ready")
		case <-ctx.Done():
			slog.Info(CMD_NAME, "status", "done")
			break Loop
		}
	}
}

// TODO Move this to the node supervisor
func newInputReader(c config.NodeConfig) inputreader.InputReader {
	return inputreader.InputReader{
		//   Model:              model,
		Provider:            c.BlockchainWsEndpoint.Value,
		InputBoxAddress:     common.HexToAddress(c.ContractsInputBoxAddress),
		InputBoxBlockNumber: uint64(c.ContractsInputBoxDeploymentBlockNumber),
		ApplicationAddress:  common.HexToAddress(c.ContractsApplicationAddress),
	}
}

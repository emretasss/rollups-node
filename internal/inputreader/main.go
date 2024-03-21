// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"time"

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
	mode        int
	rpcEndpoint string
	startBlock  int
	timeout     int
	verboseLog  bool
)

func init() {
	Cmd.Flags().IntVarP(&mode,
		"mode",
		"m",
		MODE_POLLING,
		"reading mode")
	Cmd.Flags().StringVarP(&rpcEndpoint,
		"rpc-endpoint",
		"r",
		LOCALHOST_URL,
		"RCP endpopint to be queried for inputs")
	Cmd.Flags().IntVarP(&startBlock,
		"start-block",
		"s",
		20,
		"starting block number")
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
	err := Cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	info("program timeout: %vs", timeout)

	switch mode {
	case MODE_POLLING:
		pollForInputs(ctx, rpcEndpoint, big.NewInt(20))
	case MODE_BLOCK_SUBSCRIPTION:
		mostRecentFinalizedBlock := new(BlockRef)
		mostRecentFinalizedBlock.Set(big.NewInt(20))
		subscribeForBlocks(ctx, rpcEndpoint, mostRecentFinalizedBlock)
	}
	info("done")
}

func info(format string, a ...any) (n int, err error) {
	return fmt.Printf("info> "+format+"\n", a...)
}

func debug(format string, a ...any) (n int, err error) {
	if verboseLog {
		return fmt.Printf("debug> "+format+"\n", a...)
	}
	return 0, nil
}

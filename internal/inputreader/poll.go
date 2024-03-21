package main

import (
	"context"
	"log"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

func pollForInputs(ctx context.Context, url string, startingBlockNumber *big.Int) {
	client, err := ethclient.DialContext(ctx, url)
	if err != nil {
		log.Fatalf("could not connect to backend: %v", err)
	}
	debug("client connected to %s", url)

	inputBox, err := contracts.NewInputBox(addresses.GetTestBook().InputBox, client)
	if err != nil {
		log.Fatalf("could not retrieve address for InputBox: %v", err)
	}
	debug("contract bind succeeded")

	iteratorChannel := make(chan *contracts.InputBoxInputAddedIterator)
	// Start polling goroutine
	go func() {
		info("polling started")
		fromBlockNumber := startingBlockNumber
		toBlockNumber := startingBlockNumber
		filterOpts := new(bind.FilterOpts)
		filterOpts.Context = ctx

		for {
			// XXX For some reason, infura's https endpoint always return a nil header
			mostRecentFinalizedBlock, err :=
				client.HeaderByNumber(
					ctx,
					new(big.Int).SetInt64(rpc.FinalizedBlockNumber.Int64()))

			switch {
			case err != nil:
				info("failed to retrieve most recent finalized block. %v", err)
			case mostRecentFinalizedBlock == nil:
				info("most recent finalized block was invalid")
			case mostRecentFinalizedBlock.Number.Cmp(toBlockNumber) == 0:
				info("most recent finalized block still the same (%v)", toBlockNumber)
			default:
				//TODO check whether from is less than to or let query function fail?
				filterOpts.Start = fromBlockNumber.Uint64()

				toBlockNumber = mostRecentFinalizedBlock.Number
				toBlockUint64 := toBlockNumber.Uint64()
				filterOpts.End = &toBlockUint64 // XXX

				if err := getInputs(inputBox, filterOpts, iteratorChannel); err != nil {
					// TODO Use an error channel that, depending on the mode, could trigger a retry or else
					log.Fatalf("could not get inputs: %v", err)
				}
				fromBlockNumber.Add(toBlockNumber, common.Big1)
			}
			time.Sleep(RETRY_INTERVAL * time.Second)
		}
	}()

	for {
		select {
		case iterator := <-iteratorChannel:
			defer iterator.Close()
			i := 0
			for iterator.Next() {
				debug("event[%v] = {dapp: %v, inputIndex: %v, sender: %v, input: %v}",
					i,
					iterator.Event.Dapp,
					iterator.Event.InputIndex,
					iterator.Event.Sender,
					iterator.Event.Input)
				i += 1
			}
			info("found %v events", i)
		case <-ctx.Done():
			info("finished")
			return
		}
	}
}

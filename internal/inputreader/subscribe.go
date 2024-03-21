package main

import (
	"context"
	"log"
	"time"

	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func subscribeForBlocks(ctx context.Context, url string, startingBlock *BlockRef) {
	// Boilerplate
	client, err := ethclient.DialContext(ctx, url)
	if err != nil {
		log.Fatalf("could not connect to backend: %v", err)
	}
	debug("client connected to %s", url)

	inputBox, err := contracts.NewInputBox(addresses.GetTestBook().InputBox, client)
	if err != nil {
		log.Fatal(err)
	}
	debug("contract bind succeeded")

	// Retrieve initial data
	/*
		1. start-up (goroutine)
		  - query for the most recent finalized block (LFB)
		  - get Logs from genesis to LFB
		  - latest known finalized block (LKFB) = LFB
		  - process inputs included in logs
	*/
	// 2 slots in the channel buffer are enough

	iteratorChannel := make(chan *contracts.InputBoxInputAddedIterator, 2)
	go func() {
		debug("retrieving initial data")
		fromBlockNumber := startingBlock.Number()
		toBlockNumber := startingBlock.Number()
		filterOpts := new(bind.FilterOpts)
		filterOpts.Context = ctx

		for {
			mostRecentFinalizedBlock, err := getMostRecentFinalizedHeader(ctx, client)
			if err != nil {
				info("failed to retrieve most recent finalized block. %v", err)
				continue
			}

			toBlockNumber = mostRecentFinalizedBlock.Number

			filterOpts.Start = fromBlockNumber.Uint64()
			toBlockUint64 := toBlockNumber.Uint64()
			filterOpts.End = &toBlockUint64 // XXX

			if err := getInputs(inputBox, filterOpts, iteratorChannel); err != nil {
				// TODO Use an error channel that, depending on the mode, could trigger a retry or else
				log.Fatalf("could not retrieve inputs between blocks %v and %v: %v", filterOpts.Start, filterOpts.End, err)
			}

			fromBlockNumber.Add(toBlockNumber, common.Big1)
			startingBlock.Set(fromBlockNumber)
			break
		}
	}()

	/*
		New heads have the caveat that new blocks might be excluded by re-orgs.
		In order to circumvent re-orgs, we may get the LFB whenever a new block arrives, regardless of its contents.
		This might double the amount of traffic but will assure we have the latest valid data from L1.

		2. subscribe to NewHeads (goroutine) and:
		  - whenever a block is received:
		    - query for LFB
			- if LFB > LKFB necessary:
		    	- get Logs from LKFB to LFB
		    	- LKFB = LFB
		    	- process inputs included in logs
	*/

	headers := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(ctx, headers)
	if err != nil {
		log.Fatalf("could not start subscription: %v", err)
	}
	debug("subscription started")

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("subscription failed: %v", err)
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
			// TODO Send events to be processed

		case header := <-headers:

			if verboseLog {
				block, err := client.BlockByHash(context.Background(), header.Hash())
				if err != nil {
					log.Fatalf("could not retrieve block data for header %v, header.Hash(): %v", err)
				}
				debug("received notification about block number %v", block.Number().Uint64())
			}
			// Ignore header and retrieve finalized blocks
			fromBlockNumber := startingBlock.Number()
			toBlockNumber := startingBlock.Number()
			filterOpts := new(bind.FilterOpts)
			filterOpts.Context = ctx

			// XXX Come up with a resilient way and perhaps use a backoff mechanism
			// for querying data from L1 instead of using an endless loop
		QueryLoop:
			for {
				mostRecentFinalizedBlock, err := getMostRecentFinalizedHeader(ctx, client)
				switch {
				case err != nil:
					info("failed to retrieve most recent finalized block. %v", err)
					time.Sleep(RETRY_INTERVAL * time.Second)
					continue

				case mostRecentFinalizedBlock.Number.Cmp(toBlockNumber) == 0:
					info("most recent finalized block still the same (%v)", toBlockNumber)
					break QueryLoop

				default:
					toBlockNumber = mostRecentFinalizedBlock.Number
					info("most recent finalized block: %v", toBlockNumber)

					filterOpts.Start = fromBlockNumber.Uint64()
					toBlockUint64 := toBlockNumber.Uint64()
					filterOpts.End = &toBlockUint64 // XXX

					if err := getInputs(inputBox, filterOpts, iteratorChannel); err != nil {
						// TODO Use an error channel that, depending on the mode, could trigger a retry or else
						log.Fatalf("could not get inputs: %v", err)
					}

					fromBlockNumber.Add(toBlockNumber, common.Big1)
					startingBlock.Set(fromBlockNumber)
					break
				}
			}

		case <-ctx.Done():
			info("finished")
			return
		}
	}
}

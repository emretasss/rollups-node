package main

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	"github.com/cartesi/rollups-node/pkg/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

func getMostRecentFinalizedHeader(ctx context.Context, client *ethclient.Client) (*types.Header, error) {
	// XXX For some reason, infura's https endpoint always return a nil header
	block, err :=
		client.HeaderByNumber(
			ctx,
			new(big.Int).SetInt64(rpc.FinalizedBlockNumber.Int64()))

	if err != nil {
		return nil, err
	}

	if block == nil {
		return nil, fmt.Errorf("nil block")
	}

	return block, nil
}

// TODO Improve concurrency
var queryMutex sync.Mutex

func getInputs(inputBox *contracts.InputBox, filterOpts *bind.FilterOpts, iteratorChannel chan<- *contracts.InputBoxInputAddedIterator) error {
	queryMutex.Lock()
	defer queryMutex.Unlock()
	defer debug("finished querying blocks %v to %v", filterOpts.Start, *filterOpts.End)

	info("querying blocks %v to %v", filterOpts.Start, *filterOpts.End)
	// Query all DApps
	iterator, err := inputBox.InputBoxFilterer.FilterInputAdded(filterOpts, nil, nil)
	if err != nil {
		return err
	}

	iteratorChannel <- iterator
	return nil
}

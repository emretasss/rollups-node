package inputreader

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/pkg/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"golang.org/x/exp/slog"
)

/////// TO BE REMOVED

func info(format string, a ...any) (n int, err error) {
	return fmt.Printf("info> "+format+"\n", a...)
}

func debug(format string, a ...any) (n int, err error) {
	return fmt.Printf("debug> "+format+"\n", a...)
}

///////

type Repository interface {
	InsertInput(
		index uint64,
		payload []byte,
		status CompletionStatus,
	)
}

// InputReader reads inputs from the blockchain
type InputReader struct {
	Provider              string
	InputBoxAddress       common.Address
	InputBoxBlockNumber   uint64
	ApplicationAddress    common.Address
	MostRecentBlockNumber uint64
	Repository            Repository
}

func (r InputReader) String() string {
	return "input-reader"
}

func (r InputReader) Start(
	ctx context.Context,
	ready chan<- struct{},
) error {
	// Instantiate http client against WS endpoint
	// TODO consider moving part of the code to pkg/ethutil
	client, err := ethclient.DialContext(ctx, r.Provider)
	if err != nil {
		return fmt.Errorf("failed to connect to provider: %v", err)
	}
	debug("eth client connected")

	inputBox, err := contracts.NewInputBox(r.InputBoxAddress, client)
	if err != nil {
		log.Fatal(err)
	}
	debug("bound to InputBox")

	ready <- struct{}{}

	mostRecentFinalizedHeader, err := fetchMostRecentFinalizedHeader(ctx, client)
	if err != nil {
		return err
	}
	r.MostRecentBlockNumber = mostRecentFinalizedHeader.Number.Uint64()

	opts := bind.FilterOpts{
		Context: ctx,
		Start:   r.InputBoxBlockNumber,
		End:     &r.MostRecentBlockNumber,
	}

	err = r.readInputs(ctx, client, inputBox, &opts)
	if err != nil {
		return err
	}

	return r.watchNewBlocks(ctx, client, inputBox)
}

// Fetch the most recent `finalized` header, up to what all inputs should be
// considered finalized in L1
func fetchMostRecentFinalizedHeader(
	ctx context.Context,
	client *ethclient.Client,
) (*types.Header, error) {
	header, err :=
		client.HeaderByNumber(
			ctx,
			new(big.Int).SetInt64(rpc.FinalizedBlockNumber.Int64()))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve most recent finalized header. %v", err)
	}

	if header == nil {
		return nil, fmt.Errorf("returned header is nil")
	}

	return header, nil
}

// Read inputs given specific filter options.
func (r InputReader) readInputs(
	ctx context.Context,
	client *ethclient.Client,
	inputBox *contracts.InputBox,
	opts *bind.FilterOpts,
) error {
	filter := []common.Address{r.ApplicationAddress}
	it, err := inputBox.FilterInputAdded(opts, filter, nil)
	if err != nil {
		return fmt.Errorf("failed to read inputs from block %v to block %v. %v", opts.Start, opts.End, err)
	}
	defer it.Close()
	/*
	   for it.Next() {
	   		if err := r.addInput(ctx, client, it.Event); err != nil {
	   			return err
	   		}
	   	}
	*/
	debug("inputs from block %v to block %v retrieved", opts.Start, *opts.End)
	return nil
}

// Watch for new blocks and reads new inputs from finalized block which have not
// been processed yet.
// This function continues to run forever until there is an error or the context
// is canceled.
func (r InputReader) watchNewBlocks(
	ctx context.Context,
	client *ethclient.Client,
	inputBox *contracts.InputBox,
) error {
	headers := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(ctx, headers)
	if err != nil {
		return fmt.Errorf("could not initiate subscription: %v", err)
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-sub.Err():
			return fmt.Errorf("subscription failed: %v", err)
		case header := <-headers:
			block, err := client.BlockByHash(context.Background(), header.Hash())
			if err != nil {
				return fmt.Errorf("could not retrieve block %v: %v", header.Hash(), err)
			}
			debug("received notification about block number %v", block.Number().Uint64())

			mostRecentFinalizedHeader, err := fetchMostRecentFinalizedHeader(ctx, client)
			switch {
			case err != nil:
				return fmt.Errorf("failed to retrieve most recent finalized block. %v", err)

			case r.MostRecentBlockNumber == mostRecentFinalizedHeader.Number.Uint64():
				debug("most recent finalized block still the same(%v)", r.MostRecentBlockNumber)
				continue

			default:
				// XXX Come up with a better name
				mostRecentHeaderNumber := mostRecentFinalizedHeader.Number.Uint64()
				opts := bind.FilterOpts{
					Context: ctx,
					Start:   r.MostRecentBlockNumber + 1,
					End:     &mostRecentHeaderNumber,
				}
				err = r.readInputs(ctx, client, inputBox, &opts)
				if err != nil {
					return err
				}

				r.MostRecentBlockNumber = mostRecentFinalizedHeader.Number.Uint64()
			}
		}
	}
}

// Add the input to the model.
func (r InputReader) addInput(
	ctx context.Context,
	client *ethclient.Client,
	event *contracts.InputBoxInputAdded,
) error {
	header, err := client.HeaderByHash(ctx, event.Raw.BlockHash)
	if err != nil {
		return fmt.Errorf("inputter: failed to get tx header: %w", err)
	}
	timestamp := time.Unix(int64(header.Time), 0)
	slog.Debug("inputter: read event",
		"dapp", event.Dapp,
		"input.index", event.InputIndex,
		"sender", event.Sender,
		"input", event.Input,
		slog.Group("block",
			"number", header.Number,
			"timestamp", timestamp,
		),
	)
	r.Model.AddAdvanceInput(
		event.Sender,
		event.Input,
		event.Raw.BlockNumber,
		timestamp,
	)
	return nil
}

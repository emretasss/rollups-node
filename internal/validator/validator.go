// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// ------------------------------------------------------------------------------------------------
// Types
// ------------------------------------------------------------------------------------------------

type Validator struct {
	repo                    ValidatorRepository
	epochDuration           uint64
	inputBoxDeploymentBlock uint64
}

type ValidatorRepository interface {
	// GetMostRecentBlock returns the number of the last finalized block
	// processed by InputReader
	GetMostRecentBlock(ctx context.Context) (uint64, error)
	// GetCurrentEpoch returns the current epoch
	GetCurrentEpoch(ctx context.Context) (Epoch, error)
	// GetMachineStateHash returns the hash of the state of the Cartesi Machine
	// after processing the input at `inputIndex`
	GetMachineStateHash(ctx context.Context, inputIndex uint64) ([]byte, error)
	// GetAllOutputsFromProcessedInputs returns an ordered slice of all outputs
	// produced by inputs sent between `startBlock` and `endBlock` (inclusive).
	// If one or more inputs are still unprocessed,
	// it waits for its status to change until a timeout is reached
	GetAllOutputsFromProcessedInputs(
		ctx context.Context,
		startBlock uint64,
		endBlock uint64,
		timeout *time.Duration,
	) ([]Output, error)
	// InsertFirstEpochTransaction performs a database transaction
	// containing two operations:
	//
	// 1. Inserts an Epoch
	//
	// 2. Updates the current Epoch to the newly created Epoch
	//
	// This should only be called once to set up the first Epoch in the database.
	// If there's a current Epoch already, this call will have no effect
	InsertFirstEpochTransaction(
		ctx context.Context,
		epoch Epoch,
	) error
	// FinishEmptyEpochTransaction performs a database transaction
	// containing two operations:
	//
	// 	1. Inserts a new Epoch
	//
	// 	2. Updates the current Epoch to the newly created Epoch
	FinishEmptyEpochTransaction(ctx context.Context, nextEpoch Epoch) error
	// FinishEpochTransaction performs a database transaction
	// containing four operations:
	//
	// 	1. Inserts a new Epoch
	//
	// 	2. Updates the current Epoch to the newly created Epoch
	//
	// 	3. Inserts a new Claim
	//
	//	4. Inserts all the proofs from the last Epoch
	FinishEpochTransaction(
		ctx context.Context,
		newEpoch Epoch,
		claim *Claim,
		proofs []Proof,
	) error
}

type InputRange struct {
	first uint64
	last  uint64
}

// FIXME: the types below will probably be imported from somewhere else

type Epoch struct {
	StartBlock uint64
	EndBlock   uint64
}

type Claim struct {
	FirstInputIndex uint64
	LastInputIndex  uint64
	EpochHash       []byte
}

type Output struct {
	// Input whose processing produced the output
	InputIndex uint64 `json:"inputIndex"`
	// Output index within the context of the input that produced it
	Index uint64 `json:"index"`
	// Output data as a blob, starting with '0x'
	Blob []byte `json:"blob"`
}

type Proof struct {
	FirstInputIndex                  uint64
	LastInputIndex                   uint64
	InputIndexWithinEpoch            uint64
	OutputIndexWithinInput           uint64
	OutputHashesRootHash             []byte
	OutputsEpochRootHash             []byte
	MachineStateHash                 []byte
	OutputHashInOutputHashesSiblings [][]byte
	OutputHashesInEpochSiblings      [][]byte
}

// ------------------------------------------------------------------------------------------------
// Service implementation
// ------------------------------------------------------------------------------------------------

func (v Validator) String() string {
	return "validator"
}

func (v Validator) Start(ctx context.Context, ready chan<- struct{}) error {
	// create and attempt to insert the first epoch
	epoch := Epoch{v.inputBoxDeploymentBlock, v.inputBoxDeploymentBlock + v.epochDuration}
	if err := v.repo.InsertFirstEpochTransaction(ctx, epoch); err != nil {
		return err
	}
	ready <- struct{}{}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			//TODO: backoff?
			// the current epoch might be the one we just created if this is the first run
			// or another one if the Validator was restarted
			currentEpoch, err := v.repo.GetCurrentEpoch(ctx)
			if err != nil {
				return err
			}
			latestBlock, err := v.repo.GetMostRecentBlock(ctx)
			if err != nil {
				return err
			}
			// test if should finish epoch
			if latestBlock >= currentEpoch.EndBlock {
				outputs, err := v.repo.GetAllOutputsFromProcessedInputs(
					ctx,
					currentEpoch.StartBlock,
					currentEpoch.EndBlock,
					nil,
				)
				if err != nil {
					return err
				}
				newEpoch := Epoch{
					currentEpoch.EndBlock + 1,
					currentEpoch.EndBlock + v.epochDuration,
				}
				if len(outputs) == 0 {
					err := v.repo.FinishEmptyEpochTransaction(ctx, newEpoch)
					if err != nil {
						return err
					}
				} else {
					lastIndex := len(outputs) - 1
					inputRange := InputRange{outputs[0].InputIndex, outputs[lastIndex].InputIndex}
					machineStateHash, err := v.repo.GetMachineStateHash(ctx, inputRange.last)
					if err != nil {
						return err
					}
					proofs, outputsEpochRootHash, err := v.generateProofs(
						ctx,
						inputRange,
						machineStateHash,
						outputs,
					)
					if err != nil {
						return err
					}

					epochHash := crypto.Keccak256(outputsEpochRootHash, machineStateHash)
					if err = v.repo.FinishEpochTransaction(
						ctx,
						newEpoch,
						&Claim{
							FirstInputIndex: inputRange.first,
							LastInputIndex:  inputRange.last,
							EpochHash:       epochHash,
						},
						proofs,
					); err != nil {
						return err
					}
				}
			}
		}
	}
}

// ------------------------------------------------------------------------------------------------
// Validator functions
// ------------------------------------------------------------------------------------------------

func NewValidator(
	repo ValidatorRepository,
	epochDuration uint64,
	deploymentBlock uint64,
) Validator {
	return Validator{repo, epochDuration, deploymentBlock}
}

// inputIndexWithinEpoch returns the index of an input in an epoch,
// given its global index and the index of the epoch's first input
// TODO: this will probably live in the proof generating module. Maybe not as a function
func inputIndexWithinEpoch(inputIndex, epochFirstInputIndex uint64) (uint64, error) {
	if inputIndex < epochFirstInputIndex {
		return 0, fmt.Errorf("input index is not in the same epoch")
	}
	return inputIndex - epochFirstInputIndex, nil
}

// generateProofs will create the proofs for all Outputs within an Epoch.
// It returns the proofs and the root hash of the Merkle tree used to generate them
// TODO: call the proof generating module
func (v Validator) generateProofs(
	ctx context.Context,
	inputRange InputRange,
	machineStateHash []byte,
	outputs []Output,
) ([]Proof, []byte, error) {
	proofs := make([]Proof, 0, len(outputs))
	for range outputs {
		proofs = append(proofs, Proof{})
	}
	return proofs, common.Hex2Bytes("0xdeadbeef"), nil
}

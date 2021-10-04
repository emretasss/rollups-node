pub mod types;
pub use setup::{create_descartes_state_fold, DescartesStateFold, SetupConfig};

pub mod descartesv2_delegate;
mod epoch_delegate;
mod input_contract_address_delegate;
pub mod input_delegate;
pub mod output_delegate;

mod accumulating_epoch_delegate;
mod finalized_epoch_delegate;
mod sealed_epoch_delegate;

pub mod setup;

use accumulating_epoch_delegate::AccumulatingEpochFoldDelegate;
use descartesv2_delegate::DescartesV2FoldDelegate;
use epoch_delegate::EpochFoldDelegate;
use finalized_epoch_delegate::FinalizedEpochFoldDelegate;
use input_contract_address_delegate::InputContractAddressFoldDelegate;
use input_delegate::InputFoldDelegate;
use output_delegate::OutputFoldDelegate;
use sealed_epoch_delegate::SealedEpochFoldDelegate;

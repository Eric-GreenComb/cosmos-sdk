package keeper

import (
	"bytes"
	"fmt"

	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/stake/types"
)

//_________________________________________________________________________
// Accumulated updates to the active/bonded validator set for tendermint

// get the most recently updated validators
//
// CONTRACT: Only validators with non-zero power or zero-power that were bonded
// at the previous block height or were removed from the validator set entirely
// are returned to Tendermint.
func (k Keeper) GetValidTendermintUpdates(ctx sdk.Context) (updates []abci.Validator) {

	last := fetchOldValidatorSet()
	tendermintUpdates := make(map[sdk.ValAddress]uint64)

	for _, validator := range topvalidator { //(iterate(top hundred)) {
		switch validator.State {
		case Unbonded:
			unbondedToBonded(ctx, validator.Addr)
			tendermintUpdates[validator.Addr] = validator.Power
		case Unbonding:
			unbondingToBonded(ctx, validator.Addr)
			tendermintUpdates[validator.Addr] = validator.Power
		case Bonded: // do nothing
			store.delete(last[validator.Addr])
			// jailed validators are ranked last, so if we get to a jailed validator
			// we have no more bonded validators
			if validator.Jailed {
				break
			}
		}
	}

	for _, validator := range previousValidators {
		bondedToUnbonding(ctx, validator.Addr)
		tendermintUpdates[validator.Addr] = 0
	}

	return tendermintUpdates
}

//___________________________________________________________________________
// State transitions

func (ctx sdk.Context, addr sdk.ValAddress) bondedToUnbonding() {
	// perform appropriate store updates
}

func (ctx sdk.Context, addr sdk.ValAddress) unbondingToBonded() {
	// perform appropriate store updates
}

func (ctx sdk.Context, addr sdk.ValAddress) unbondedToBonded() {
	// perform appropriate store updates
}

func (ctx sdk.Context, addr sdk.ValAddress) unbondingToUnbonded() {
	// perform appropriate store updates
}

func (ctx sdk.Context, addr sdk.ValAddress) jailValidator() {
	// perform appropriate store updates
}

// XXX Delete this reference function before merge
// Perform all the necessary steps for when a validator changes its power. This
// function updates all validator stores as well as tendermint update store.
// It may kick out validators if a new validator is entering the bonded validator
// group.
//
// TODO: Remove above nolint, function needs to be simplified!
func (k Keeper) REFERENCEXXXDELETEUpdateValidator(ctx sdk.Context, validator types.Validator) types.Validator {
	tstore := ctx.TransientStore(k.storeTKey)
	pool := k.GetPool(ctx)
	oldValidator, oldFound := k.GetValidator(ctx, validator.OperatorAddr)

	validator = k.updateForJailing(ctx, oldFound, oldValidator, validator)
	powerIncreasing := k.getPowerIncreasing(ctx, oldFound, oldValidator, validator)
	validator.BondHeight, validator.BondIntraTxCounter = k.bondIncrement(ctx, oldFound, oldValidator)
	valPower := k.updateValidatorPower(ctx, oldFound, oldValidator, validator, pool)
	cliffPower := k.GetCliffValidatorPower(ctx)
	cliffValExists := (cliffPower != nil)
	var valPowerLTcliffPower bool
	if cliffValExists {
		valPowerLTcliffPower = (bytes.Compare(valPower, cliffPower) == -1)
	}

	switch {

	// if the validator is already bonded and the power is increasing, we need
	// perform the following:
	// a) update Tendermint
	// b) check if the cliff validator needs to be updated
	case powerIncreasing && !validator.Jailed &&
		(oldFound && oldValidator.Status == sdk.Bonded):

		bz := k.cdc.MustMarshalBinary(validator.ABCIValidator())
		tstore.Set(GetTendermintUpdatesTKey(validator.OperatorAddr), bz)

		if cliffValExists {
			cliffAddr := sdk.ValAddress(k.GetCliffValidator(ctx))
			if bytes.Equal(cliffAddr, validator.OperatorAddr) {
				k.updateCliffValidator(ctx, validator)
			}
		}

	// if is a new validator and the new power is less than the cliff validator
	case cliffValExists && !oldFound && valPowerLTcliffPower:
		// skip to completion

		// if was unbonded and the new power is less than the cliff validator
	case cliffValExists &&
		(oldFound && oldValidator.Status == sdk.Unbonded) &&
		valPowerLTcliffPower: //(valPower < cliffPower
		// skip to completion

	default:
		// default case - validator was either:
		//  a) not-bonded and now has power-rank greater than  cliff validator
		//  b) bonded and now has decreased in power

		// update the validator set for this validator
		updatedVal, updated := k.UpdateBondedValidators(ctx, validator)
		if updated {
			// the validator has changed bonding status
			validator = updatedVal
			break
		}

		// if decreased in power but still bonded, update Tendermint validator
		if oldFound && oldValidator.BondedTokens().GT(validator.BondedTokens()) {
			bz := k.cdc.MustMarshalBinary(validator.ABCIValidator())
			tstore.Set(GetTendermintUpdatesTKey(validator.OperatorAddr), bz)
		}
	}

	k.SetValidator(ctx, validator)
	return validator
}

func (k Keeper) updateForJailing(ctx sdk.Context, oldFound bool, oldValidator, newValidator types.Validator) types.Validator {
	if newValidator.Jailed && oldFound && oldValidator.Status == sdk.Bonded {
		newValidator = k.beginUnbondingValidator(ctx, newValidator)

		// need to also clear the cliff validator spot because the jail has
		// opened up a new spot which will be filled when
		// updateValidatorsBonded is called
		k.clearCliffValidator(ctx)
	}
	return newValidator
}

// nolint: unparam
func (k Keeper) getPowerIncreasing(ctx sdk.Context, oldFound bool, oldValidator, newValidator types.Validator) bool {
	if oldFound && oldValidator.BondedTokens().LT(newValidator.BondedTokens()) {
		return true
	}
	return false
}

// get the bond height and incremented intra-tx counter
// nolint: unparam
func (k Keeper) bondIncrement(
	ctx sdk.Context, found bool, oldValidator types.Validator) (height int64, intraTxCounter int16) {

	// if already a validator, copy the old block height and counter
	if found && oldValidator.Status == sdk.Bonded {
		height = oldValidator.BondHeight
		intraTxCounter = oldValidator.BondIntraTxCounter
		return
	}

	height = ctx.BlockHeight()
	counter := k.GetIntraTxCounter(ctx)
	intraTxCounter = counter

	k.SetIntraTxCounter(ctx, counter+1)
	return
}

func (k Keeper) updateValidatorPower(ctx sdk.Context, oldFound bool, oldValidator,
	newValidator types.Validator, pool types.Pool) (valPower []byte) {
	store := ctx.KVStore(k.storeKey)

	// update the list ordered by voting power
	if oldFound {
		store.Delete(GetValidatorsByPowerIndexKey(oldValidator, pool))
	}
	valPower = GetValidatorsByPowerIndexKey(newValidator, pool)
	store.Set(valPower, newValidator.OperatorAddr)

	return valPower
}

// XXX Delete this reference function before merge
// Update the bonded validator group based on a change to the validator
// affectedValidator. This function potentially adds the affectedValidator to
// the bonded validator group which kicks out the cliff validator. Under this
// situation this function returns the updated affectedValidator.
//
// The correct bonded subset of validators is retrieved by iterating through an
// index of the validators sorted by power, stored using the
// ValidatorsByPowerIndexKey.  Simultaneously the current validator records are
// updated in store with the ValidatorsBondedIndexKey. This store is used to
// determine if a validator is a validator without needing to iterate over all
// validators.
func (k Keeper) XXXREFERENCEUpdateBondedValidators(
	ctx sdk.Context, affectedValidator types.Validator) (
	updatedVal types.Validator, updated bool) {

	store := ctx.KVStore(k.storeKey)

	oldCliffValidatorAddr := k.GetCliffValidator(ctx)
	maxValidators := k.GetParams(ctx).MaxValidators
	bondedValidatorsCount := 0
	var validator, validatorToBond types.Validator
	newValidatorBonded := false

	// create a validator iterator ranging from largest to smallest by power
	iterator := sdk.KVStoreReversePrefixIterator(store, ValidatorsByPowerIndexKey)
	for ; iterator.Valid() && bondedValidatorsCount < int(maxValidators); iterator.Next() {

		// either retrieve the original validator from the store, or under the
		// situation that this is the "affected validator" just use the
		// validator provided because it has not yet been updated in the store
		ownerAddr := iterator.Value()
		if bytes.Equal(ownerAddr, affectedValidator.OperatorAddr) {
			validator = affectedValidator
		} else {
			var found bool
			validator = k.mustGetValidator(ctx, ownerAddr)
		}

		// if we've reached jailed validators no further bonded validators exist
		if validator.Jailed {
			if validator.Status == sdk.Bonded {
				panic(fmt.Sprintf("jailed validator cannot be bonded, address: %X\n", ownerAddr))
			}

			break
		}

		// increment the total number of bonded validators and potentially mark
		// the validator to bond
		if validator.Status != sdk.Bonded {
			validatorToBond = validator
			if newValidatorBonded {
				panic("already decided to bond a validator, can't bond another!")
			}
			newValidatorBonded = true
		}

		bondedValidatorsCount++
	}

	iterator.Close()

	if newValidatorBonded && bytes.Equal(oldCliffValidatorAddr, validator.OperatorAddr) {
		panic("cliff validator has not been changed, yet we bonded a new validator")
	}

	// clear or set the cliff validator
	if bondedValidatorsCount == int(maxValidators) {
		k.setCliffValidator(ctx, validator, k.GetPool(ctx))
	} else if len(oldCliffValidatorAddr) > 0 {
		k.clearCliffValidator(ctx)
	}

	// swap the cliff validator for a new validator if the affected validator
	// was bonded
	if newValidatorBonded {
		if oldCliffValidatorAddr != nil {
			oldCliffVal := k.mustGetValidator(ctx, oldCliffValidatorAddr)

			if bytes.Equal(validatorToBond.OperatorAddr, affectedValidator.OperatorAddr) {

				// begin unbonding the old cliff validator iff the affected
				// validator was newly bonded and has greater power
				k.beginUnbondingValidator(ctx, oldCliffVal)
			} else {
				// otherwise begin unbonding the affected validator, which must
				// have been kicked out
				affectedValidator = k.beginUnbondingValidator(ctx, affectedValidator)
			}
		}

		validator = k.bondValidator(ctx, validatorToBond)
		if bytes.Equal(validator.OperatorAddr, affectedValidator.OperatorAddr) {
			return validator, true
		}

		return affectedValidator, true
	}

	return types.Validator{}, false
}

// XXX Delete this reference function before merge
// full update of the bonded validator set, many can be added/kicked
func (k Keeper) XXXREFERENCEUpdateBondedValidatorsFull(ctx sdk.Context) {
	store := ctx.KVStore(k.storeKey)

	// clear the current validators store, add to the ToKickOut temp store
	toKickOut := make(map[string]byte)
	iterator := sdk.KVStorePrefixIterator(store, ValidatorsBondedIndexKey)
	for ; iterator.Valid(); iterator.Next() {
		ownerAddr := GetAddressFromValBondedIndexKey(iterator.Key())
		toKickOut[string(ownerAddr)] = 0
	}

	iterator.Close()

	var validator types.Validator

	oldCliffValidatorAddr := k.GetCliffValidator(ctx)
	maxValidators := k.GetParams(ctx).MaxValidators
	bondedValidatorsCount := 0

	iterator = sdk.KVStoreReversePrefixIterator(store, ValidatorsByPowerIndexKey)
	for ; iterator.Valid() && bondedValidatorsCount < int(maxValidators); iterator.Next() {
		var found bool

		ownerAddr := iterator.Value()
		validator = k.mustGetValidator(ctx, ownerAddr)

		_, found = toKickOut[string(ownerAddr)]
		if found {
			delete(toKickOut, string(ownerAddr))
		} else {
			// If the validator wasn't in the toKickOut group it means it wasn't
			// previously a validator, therefor update the validator to enter
			// the validator group.
			validator = k.bondValidator(ctx, validator)
		}

		if validator.Jailed {
			// we should no longer consider jailed validators as they are ranked
			// lower than any non-jailed/bonded validators
			if validator.Status == sdk.Bonded {
				panic(fmt.Sprintf("jailed validator cannot be bonded for address: %s\n", ownerAddr))
			}
			break
		}

		bondedValidatorsCount++
	}

	iterator.Close()

	// clear or set the cliff validator
	if bondedValidatorsCount == int(maxValidators) {
		k.setCliffValidator(ctx, validator, k.GetPool(ctx))
	} else if len(oldCliffValidatorAddr) > 0 {
		k.clearCliffValidator(ctx)
	}

	kickOutValidators(k, ctx, toKickOut)
	return
}

func kickOutValidators(k Keeper, ctx sdk.Context, toKickOut map[string]byte) {
	for key := range toKickOut {
		ownerAddr := []byte(key)
		validator := k.mustGetValidator(ctx, ownerAddr)
		k.beginUnbondingValidator(ctx, validator)
	}
}

// perform all the store operations for when a validator status becomes unbonded
func (k Keeper) beginUnbondingValidator(ctx sdk.Context, validator types.Validator) types.Validator {

	store := ctx.KVStore(k.storeKey)
	pool := k.GetPool(ctx)
	params := k.GetParams(ctx)

	// sanity check
	if validator.Status == sdk.Unbonded ||
		validator.Status == sdk.Unbonding {
		panic(fmt.Sprintf("should not already be unbonded or unbonding, validator: %v\n", validator))
	}

	// set the status
	validator, pool = validator.UpdateStatus(pool, sdk.Unbonding)
	k.SetPool(ctx, pool)

	validator.UnbondingMinTime = ctx.BlockHeader().Time.Add(params.UnbondingTime)
	validator.UnbondingHeight = ctx.BlockHeader().Height

	// save the now unbonded validator record
	k.SetValidator(ctx, validator)

	// add to accumulated changes for tendermint
	bzABCI := k.cdc.MustMarshalBinary(validator.ABCIValidatorZero())
	tstore := ctx.TransientStore(k.storeTKey)
	tstore.Set(GetTendermintUpdatesTKey(validator.OperatorAddr), bzABCI)

	// also remove from the Bonded types.Validators Store
	store.Delete(GetValidatorsBondedIndexKey(validator.OperatorAddr))

	// call the unbond hook if present
	if k.hooks != nil {
		k.hooks.OnValidatorBeginUnbonding(ctx, validator.ConsAddress())
	}

	// return updated validator
	return validator
}

// perform all the store operations for when a validator status becomes bonded
func (k Keeper) bondValidator(ctx sdk.Context, validator types.Validator) types.Validator {

	store := ctx.KVStore(k.storeKey)
	pool := k.GetPool(ctx)

	// sanity check
	if validator.Status == sdk.Bonded {
		panic(fmt.Sprintf("should not already be bonded, validator: %v\n", validator))
	}

	validator.BondHeight = ctx.BlockHeight()

	// set the status
	validator, pool = validator.UpdateStatus(pool, sdk.Bonded)
	k.SetPool(ctx, pool)

	// save the now bonded validator record to the three referenced stores
	k.SetValidator(ctx, validator)
	store.Set(GetValidatorsBondedIndexKey(validator.OperatorAddr), []byte{})

	// add to accumulated changes for tendermint
	bzABCI := k.cdc.MustMarshalBinary(validator.ABCIValidator())
	tstore := ctx.TransientStore(k.storeTKey)
	tstore.Set(GetTendermintUpdatesTKey(validator.OperatorAddr), bzABCI)

	// call the bond hook if present
	if k.hooks != nil {
		k.hooks.OnValidatorBonded(ctx, validator.ConsAddress())
	}

	// return updated validator
	return validator
}

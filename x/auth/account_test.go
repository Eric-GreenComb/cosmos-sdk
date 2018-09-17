package auth

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"

	codec "github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func keyPubAddr() (crypto.PrivKey, crypto.PubKey, sdk.AccAddress) {
	key := ed25519.GenPrivKey()
	pub := key.PubKey()
	addr := sdk.AccAddress(pub.Address())
	return key, pub, addr
}

func TestBaseAddressPubKey(t *testing.T) {
	_, pub1, addr1 := keyPubAddr()
	_, pub2, addr2 := keyPubAddr()
	acc := NewBaseAccountWithAddress(addr1)

	// check the address (set) and pubkey (not set)
	require.EqualValues(t, addr1, acc.GetAddress())
	require.EqualValues(t, nil, acc.GetPubKey())

	// can't override address
	err := acc.SetAddress(addr2)
	require.NotNil(t, err)
	require.EqualValues(t, addr1, acc.GetAddress())

	// set the pubkey
	err = acc.SetPubKey(pub1)
	require.Nil(t, err)
	require.Equal(t, pub1, acc.GetPubKey())

	// can override pubkey
	err = acc.SetPubKey(pub2)
	require.Nil(t, err)
	require.Equal(t, pub2, acc.GetPubKey())

	//------------------------------------

	// can set address on empty account
	acc2 := BaseAccount{}
	err = acc2.SetAddress(addr2)
	require.Nil(t, err)
	require.EqualValues(t, addr2, acc2.GetAddress())
}

func TestBaseAccountCoins(t *testing.T) {
	_, _, addr := keyPubAddr()
	acc := NewBaseAccountWithAddress(addr)

	someCoins := sdk.Coins{sdk.NewInt64Coin("atom", 123), sdk.NewInt64Coin("eth", 246)}

	err := acc.SetCoins(someCoins)
	require.Nil(t, err)
	require.Equal(t, someCoins, acc.GetCoins())
}

func TestBaseAccountSequence(t *testing.T) {
	_, _, addr := keyPubAddr()
	acc := NewBaseAccountWithAddress(addr)

	seq := int64(7)

	err := acc.SetSequence(seq)
	require.Nil(t, err)
	require.Equal(t, seq, acc.GetSequence())
}

func TestBaseAccountMarshal(t *testing.T) {
	_, pub, addr := keyPubAddr()
	acc := NewBaseAccountWithAddress(addr)

	someCoins := sdk.Coins{sdk.NewInt64Coin("atom", 123), sdk.NewInt64Coin("eth", 246)}
	seq := int64(7)

	// set everything on the account
	err := acc.SetPubKey(pub)
	require.Nil(t, err)
	err = acc.SetSequence(seq)
	require.Nil(t, err)
	err = acc.SetCoins(someCoins)
	require.Nil(t, err)

	// need a codec for marshaling
	cdc := codec.New()
	codec.RegisterCrypto(cdc)

	b, err := cdc.MarshalBinary(acc)
	require.Nil(t, err)

	acc2 := BaseAccount{}
	err = cdc.UnmarshalBinary(b, &acc2)
	require.Nil(t, err)
	require.Equal(t, acc, acc2)

	// error on bad bytes
	acc2 = BaseAccount{}
	err = cdc.UnmarshalBinary(b[:len(b)/2], &acc2)
	require.NotNil(t, err)
}

func TestSendableCoinsContinuousVesting(t *testing.T) {
	cases := []struct {
		blockTime        time.Time
		transferredCoins sdk.Coins
		delegatedCoins   sdk.Coins
		expectedSendable sdk.Coins
	}{
		// No tranfers
		{time.Unix(0, 0), sdk.Coins(nil), sdk.Coins(nil), sdk.Coins(nil)},                            // No coins available on initialization
		{time.Unix(500, 0), sdk.Coins(nil), sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(500)}}},   // Half coins available at halfway point
		{time.Unix(1000, 0), sdk.Coins(nil), sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(1000)}}}, // All coins available after EndTime
		{time.Unix(2000, 0), sdk.Coins(nil), sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(1000)}}}, // SendableCoins doesn't linearly increase after EndTime

		// Transfers
		{time.Unix(0, 0), sdk.Coins{{"steak", sdk.NewInt(100)}}, sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(100)}}},                                                                // Only transferred coins are sendable at time 0.
		{time.Unix(500, 0), sdk.Coins{{"photon", sdk.NewInt(1000)}, {"steak", sdk.NewInt(100)}}, sdk.Coins(nil), sdk.Coins{{"photon", sdk.NewInt(1000)}, {"steak", sdk.NewInt(600)}}},  // scheduled coins + transferred coins
		{time.Unix(500, 0), sdk.Coins{{"photon", sdk.NewInt(1000)}, {"steak", sdk.NewInt(-100)}}, sdk.Coins(nil), sdk.Coins{{"photon", sdk.NewInt(1000)}, {"steak", sdk.NewInt(400)}}}, // scheduled coins + transferred coins

		// Delegations
		{time.Unix(500, 0), sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(400)}}, sdk.Coins{{"steak", sdk.NewInt(500)}}}, // All delegated tokens are vesting
		{time.Unix(500, 0), sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(800)}}, sdk.Coins{{"steak", sdk.NewInt(200)}}}, // Some delegated tokens were unlocked (300)
		{time.Unix(1000, 0), sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(1000)}}, sdk.Coins(nil)},                      // All coins are delegated

		// Integration Tests: Transfers and Delegations
		{time.Unix(0, 0), sdk.Coins{{"photon", sdk.NewInt(10)}, {"steak", sdk.NewInt(10)}}, sdk.Coins{{"steak", sdk.NewInt(5)}}, sdk.Coins{{"photon", sdk.NewInt(10)}, {"steak", sdk.NewInt(10)}}}, // Delegate some of transferred coins
		{time.Unix(500, 0), sdk.Coins{{"steak", sdk.NewInt(10)}}, sdk.Coins{{"steak", sdk.NewInt(400)}}, sdk.Coins{{"steak", sdk.NewInt(510)}}},                                                    // Delegate locked coins only
		{time.Unix(500, 0), sdk.Coins{{"steak", sdk.NewInt(10)}}, sdk.Coins{{"steak", sdk.NewInt(800)}}, sdk.Coins{{"steak", sdk.NewInt(210)}}},                                                    // Delegate all locked coins and some unlocked coins
		{time.Unix(500, 0), sdk.Coins{{"steak", sdk.NewInt(10)}}, sdk.Coins{{"steak", sdk.NewInt(1005)}}, sdk.Coins{{"steak", sdk.NewInt(5)}}},                                                     // Delegate all locked coins and most of unlocked coins (including some transferred coins)

		{time.Unix(500, 0), sdk.Coins{{"steak", sdk.NewInt(-10)}}, sdk.Coins{{"steak", sdk.NewInt(400)}}, sdk.Coins{{"steak", sdk.NewInt(490)}}}, // Transfer unlocked coins, delegate locked coins
		{time.Unix(500, 0), sdk.Coins{{"steak", sdk.NewInt(-10)}}, sdk.Coins{{"steak", sdk.NewInt(800)}}, sdk.Coins{{"steak", sdk.NewInt(190)}}}, // Transfer unlocked coins, delegate locked and unlocked coins
		{time.Unix(500, 0), sdk.Coins{{"steak", sdk.NewInt(-10)}}, sdk.Coins{{"steak", sdk.NewInt(990)}}, sdk.Coins(nil)},                        // Transfer unlocked coins, delegate rest of account
	}

	for i, c := range cases {
		_, _, addr := keyPubAddr()
		vacc := NewContinuousVestingAccount(addr, sdk.Coins{{"steak", sdk.NewInt(1000)}}, time.Unix(0, 0), time.Unix(1000, 0))
		coins := vacc.GetCoins().Plus(c.transferredCoins)
		coins = coins.Minus(c.delegatedCoins) // delegation is not tracked
		vacc.SetCoins(coins)
		vacc.TrackTransfers(c.transferredCoins)

		sendable := vacc.SendableCoins(c.blockTime)
		require.Equal(t, c.expectedSendable, sendable, fmt.Sprintf("Expected sendablecoins is incorrect for testcase %d: {Transferred: %s, Delegated: %s, Time: %d",
			i, c.transferredCoins, c.delegatedCoins, c.blockTime.Unix()))
	}
}

func TestSendableCoinsDelayTransfer(t *testing.T) {
	cases := []struct {
		blockTime        time.Time
		transferredCoins sdk.Coins
		delegatedCoins   sdk.Coins
		expectedSendable sdk.Coins
	}{
		// No transfers or delegations
		{time.Unix(500, 0), sdk.Coins(nil), sdk.Coins(nil), sdk.Coins(nil)},                          // Before EndTime. All coins locked
		{time.Unix(1000, 0), sdk.Coins(nil), sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(1000)}}}, // At Endtime, all coins unlocked

		// Transfers
		{time.Unix(500, 0), sdk.Coins{{"steak", sdk.NewInt(100)}}, sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(100)}}},   // Transfer before EndTime
		{time.Unix(1000, 0), sdk.Coins{{"steak", sdk.NewInt(100)}}, sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(1100)}}}, // Transfer after EndTime

		// Delegations
		{time.Unix(1000, 0), sdk.Coins(nil), sdk.Coins{{"steak", sdk.NewInt(800)}}, sdk.Coins{{"steak", sdk.NewInt(200)}}}, // Some unlocked coins are delegated

		// Transfers and Delegations
		{time.Unix(500, 0), sdk.Coins{{"steak", sdk.NewInt(500)}}, sdk.Coins{{"steak", sdk.NewInt(1000)}}, sdk.Coins{{"steak", sdk.NewInt(500)}}}, // Delegate all locked coins
		{time.Unix(500, 0), sdk.Coins{{"steak", sdk.NewInt(500)}}, sdk.Coins{{"steak", sdk.NewInt(1300)}}, sdk.Coins{{"steak", sdk.NewInt(200)}}}, // Delegate all locked coins and some transferred coins
	}

	for i, c := range cases {
		_, _, addr := keyPubAddr()
		vacc := NewDelayTransferAccount(addr, sdk.Coins{{"steak", sdk.NewInt(1000)}}, time.Unix(1000, 0))
		coins := vacc.GetCoins().Plus(c.transferredCoins)
		coins = coins.Minus(c.delegatedCoins) // delegation is not tracked
		vacc.SetCoins(coins)
		vacc.TrackTransfers(c.transferredCoins)

		sendable := vacc.SendableCoins(c.blockTime)
		require.Equal(t, c.expectedSendable, sendable, fmt.Sprintf("Expected sendablecoins is incorrect for testcase %d: {Transferred: %s, Delegated: %s, Time: %d",
			i, c.transferredCoins, c.delegatedCoins, c.blockTime.Unix()))
	}
}

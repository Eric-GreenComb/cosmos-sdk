package context

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

// TODO: This should get deleted eventually, and perhaps
// ctypes.ResultBroadcastTx be stripped of unused fields, and
// ctypes.ResultBroadcastTxCommit returned for tendermint RPC BroadcastTxSync.
//
// The motivation is that we want a unified type to return, and the better
// option is the one that can hold CheckTx/DeliverTx responses optionally.
func resultBroadcastTxToCommit(res *ctypes.ResultBroadcastTx) *ctypes.ResultBroadcastTxCommit {
	return &ctypes.ResultBroadcastTxCommit{
		Hash: res.Hash,
		// NOTE: other fields are unused for async.
	}
}

// BroadcastTx broadcasts a transactions either synchronously or asynchronously
// based on the context parameters. The result of the broadcast is parsed into
// an intermediate structure which is logged if the context has a logger
// defined.
func (ctx CLIContext) BroadcastTx(txBytes []byte) (*ctypes.ResultBroadcastTxCommit, error) {
	if ctx.Async {
		res, err := ctx.broadcastTxAsync(txBytes)
		if err != nil {
			return nil, err
		}

		resCommit := resultBroadcastTxToCommit(res)
		return resCommit, err
	}

	return ctx.broadcastTxCommit(txBytes)
}

// BroadcastTxAndAwaitCommit broadcasts transaction bytes to a Tendermint node
// and waits for a commit.
func (ctx CLIContext) BroadcastTxAndAwaitCommit(tx []byte) (*ctypes.ResultBroadcastTxCommit, error) {
	node, err := ctx.GetNode()
	if err != nil {
		return nil, err
	}

	res, err := node.BroadcastTxCommit(tx)
	if err != nil {
		return res, err
	}

	if !res.CheckTx.IsOK() {
		return res, errors.Errorf("checkTx failed: (%d) %s",
			res.CheckTx.Code,
			res.CheckTx.Log)
	}

	if !res.DeliverTx.IsOK() {
		return res, errors.Errorf("deliverTx failed: (%d) %s",
			res.DeliverTx.Code,
			res.DeliverTx.Log)
	}

	return res, err
}

// BroadcastTxAsync broadcasts transaction bytes to a Tendermint node
// asynchronously.
func (ctx CLIContext) BroadcastTxAsync(tx []byte) (*ctypes.ResultBroadcastTx, error) {
	node, err := ctx.GetNode()
	if err != nil {
		return nil, err
	}

	res, err := node.BroadcastTxAsync(tx)
	if err != nil {
		return res, err
	}

	return res, err
}

func (ctx CLIContext) broadcastTxAsync(txBytes []byte) (*ctypes.ResultBroadcastTx, error) {
	res, err := ctx.BroadcastTxAsync(txBytes)
	if err != nil {
		return res, err
	}

	if ctx.Logger != nil {
		if ctx.JSON {
			type toJSON struct {
				TxHash string
			}

			resJSON := toJSON{res.Hash.String()}
			bz, err := ctx.Codec.MarshalJSON(resJSON)
			if err != nil {
				return res, err
			}

			ctx.Logger.Write(bz)
			io.WriteString(ctx.Logger, "\n")
		} else {
			io.WriteString(ctx.Logger, fmt.Sprintf("async tx sent (tx hash: %s)\n", res.Hash))
		}
	}

	return res, nil
}

func (ctx CLIContext) broadcastTxCommit(txBytes []byte) (*ctypes.ResultBroadcastTxCommit, error) {
	res, err := ctx.BroadcastTxAndAwaitCommit(txBytes)
	if err != nil {
		return res, err
	}

	if ctx.JSON {
		// Since JSON is intended for automated scripts, always include response in
		// JSON mode.
		type toJSON struct {
			Height   int64
			TxHash   string
			Response string
		}

		if ctx.Logger != nil {
			resJSON := toJSON{res.Height, res.Hash.String(), fmt.Sprintf("%+v", res.DeliverTx)}
			bz, err := ctx.Codec.MarshalJSON(resJSON)
			if err != nil {
				return res, err
			}

			ctx.Logger.Write(bz)
			io.WriteString(ctx.Logger, "\n")
		}

		return res, nil
	}

	if ctx.Logger != nil {
		resStr := fmt.Sprintf("Committed at block %d (tx hash: %s)\n", res.Height, res.Hash.String())

		if ctx.PrintResponse {
			resStr = fmt.Sprintf("Committed at block %d (tx hash: %s, response: %+v)\n",
				res.Height, res.Hash.String(), res.DeliverTx,
			)
		}

		io.WriteString(ctx.Logger, resStr)
	}

	return res, nil
}
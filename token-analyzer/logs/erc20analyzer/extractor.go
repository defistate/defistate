package erc20analyzer

import (
	"math/big"
	"time"

	"github.com/defistate/defistate/token-analyzer/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type MaxTransferRecord struct {
	Address common.Address
	Amount  *big.Int
	Time    time.Time
}

// ExtractMaxSingleTransfer finds the largest single transfer from an allowed address.
func ExtractMaxSingleTransfer(logs []types.Log, isAllowedAddress func(common.Address) bool) map[common.Address]MaxTransferRecord {
	maxTransferByToken := make(map[common.Address]MaxTransferRecord)

	for _, l := range logs {
		from, _, value, err := abi.ParseERC20TransferEvent(l)
		if err != nil {
			continue
		}

		// 💡 Only process transfers where the sender is allowed.
		if !isAllowedAddress(from) {
			continue
		}

		tokenAddress := l.Address
		currentMax, ok := maxTransferByToken[tokenAddress]

		if !ok || value.Cmp(currentMax.Amount) > 0 {
			maxTransferByToken[tokenAddress] = MaxTransferRecord{
				Address: from,
				Amount:  value,
				Time:    time.Now(),
			}
		}
	}
	return maxTransferByToken
}

// ExtractMaxTotalVolumeTransferrer finds the allowed address with the highest total volume.
func ExtractMaxTotalVolumeTransferrer(logs []types.Log, isAllowedAddress func(common.Address) bool) map[common.Address]MaxTransferRecord {
	totals := make(map[common.Address]map[common.Address]*big.Int)

	for _, l := range logs {
		from, _, value, err := abi.ParseERC20TransferEvent(l)
		if err != nil {
			continue
		}

		// 💡 Only aggregate transfers where the sender is allowed.
		if !isAllowedAddress(from) {
			continue
		}

		tokenAddress := l.Address
		if _, ok := totals[tokenAddress]; !ok {
			totals[tokenAddress] = make(map[common.Address]*big.Int)
		}
		if _, ok := totals[tokenAddress][from]; !ok {
			totals[tokenAddress][from] = new(big.Int)
		}
		totals[tokenAddress][from].Add(totals[tokenAddress][from], value)
	}

	maxTransferByToken := make(map[common.Address]MaxTransferRecord)
	for token, fromMap := range totals {
		var currentMax MaxTransferRecord
		isFirst := true
		for from, totalAmount := range fromMap {
			if isFirst || totalAmount.Cmp(currentMax.Amount) > 0 {
				currentMax = MaxTransferRecord{
					Address: from,
					Amount:  totalAmount,
					Time:    time.Now(),
				}
				isFirst = false
			}
		}
		if !isFirst {
			maxTransferByToken[token] = currentMax
		}
	}
	return maxTransferByToken
}

// ExtractMaxSingleReceiver finds the largest single transfer to an allowed address.
func ExtractMaxSingleReceiver(logs []types.Log, isAllowedAddress func(common.Address) bool) map[common.Address]MaxTransferRecord {
	maxTransferByToken := make(map[common.Address]MaxTransferRecord)

	for _, l := range logs {
		_, to, value, err := abi.ParseERC20TransferEvent(l)
		if err != nil {
			continue
		}

		// 💡 Only process transfers where the receiver is allowed.
		if !isAllowedAddress(to) {
			continue
		}

		tokenAddress := l.Address
		currentMax, ok := maxTransferByToken[tokenAddress]

		if !ok || value.Cmp(currentMax.Amount) > 0 {
			maxTransferByToken[tokenAddress] = MaxTransferRecord{
				Address: to,
				Amount:  value,
				Time:    time.Now(),
			}
		}
	}
	return maxTransferByToken
}

// ExtractMaxTotalVolumeReceiver finds the allowed address with the highest total volume received.
func ExtractMaxTotalVolumeReceiver(logs []types.Log, isAllowedAddress func(common.Address) bool) map[common.Address]MaxTransferRecord {
	totals := make(map[common.Address]map[common.Address]*big.Int)

	for _, l := range logs {
		_, to, value, err := abi.ParseERC20TransferEvent(l)
		if err != nil {
			continue
		}

		// 💡 Only aggregate transfers where the receiver is allowed.
		if !isAllowedAddress(to) {
			continue
		}

		tokenAddress := l.Address
		if _, ok := totals[tokenAddress]; !ok {
			totals[tokenAddress] = make(map[common.Address]*big.Int)
		}
		if _, ok := totals[tokenAddress][to]; !ok {
			totals[tokenAddress][to] = new(big.Int)
		}
		totals[tokenAddress][to].Add(totals[tokenAddress][to], value)
	}

	maxTransferByToken := make(map[common.Address]MaxTransferRecord)
	for token, toMap := range totals {
		var currentMax MaxTransferRecord
		isFirst := true
		for to, totalAmount := range toMap {
			if isFirst || totalAmount.Cmp(currentMax.Amount) > 0 {
				currentMax = MaxTransferRecord{
					Address: to,
					Amount:  totalAmount,
					Time:    time.Now(),
				}
				isFirst = false
			}
		}
		if !isFirst {
			maxTransferByToken[token] = currentMax
		}
	}
	return maxTransferByToken
}

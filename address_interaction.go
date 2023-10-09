package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	infuraURL  = "https://flashy-intensive-glade.base-mainnet.discover.quiknode.pro/fc937e6a917d8493f86719ba041482277cfd3e26/"
	blockRange = 10000 // Query block range, should be <= 10000
	startBlock = 2430440
	outputFile = "/mnt/data/addresses.json"
)

func main() {
	client, err := ethclient.Dial(infuraURL)
	if err != nil {
		log.Fatalf("Failed to connect to Infura: %v", err)
	}

	// Retrieve the latest block number
	latestBlock, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to get latest block: %v", err)
	}

	addressTransactions := make(map[string]int)

	// Iterate through the block ranges from the latest block to startBlock
	for fromBlock := latestBlock.NumberU64(); fromBlock > startBlock; fromBlock -= blockRange {
		toBlock := fromBlock - blockRange
		if toBlock < startBlock {
			toBlock = startBlock
		}

		log.Printf("Processing blocks from %d to %d...\n", toBlock, fromBlock)

		// Instead of filtering logs, get the block and iterate over its transactions
		for blockNum := toBlock; blockNum <= fromBlock; blockNum++ {
			block, err := client.BlockByNumber(context.Background(), big.NewInt(int64(blockNum)))
			if err != nil {
				log.Fatalf("Failed to retrieve block %d: %v", blockNum, err)
			}

			for _, tx := range block.Transactions() {
				from, err := types.Sender(types.NewEIP155Signer(tx.ChainId()), tx)
				if err != nil {
					log.Fatalf("Failed to get the sender address from transaction: %v", err)
				} // sender address
				to := tx.To() // receiver address

				// Increment the count for both sender and receiver addresses
				addressTransactions[from.Hex()]++
				if to != nil {
					addressTransactions[to.Hex()]++
				}
			}
		}
	}

	// Write the addresses and their transaction counts to a JSON file
	data, err := json.MarshalIndent(addressTransactions, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}
	err = ioutil.WriteFile(outputFile, data, 0644)
	if err != nil {
		log.Fatalf("Failed to write to file: %v", err)
	}

	log.Printf("Addresses and their transaction counts saved to %s", outputFile)
}

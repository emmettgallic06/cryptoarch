package main

import (
	"context"
	"database/sql"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	_ "github.com/lib/pq"
	"golang.org/x/time/rate"
)

var limiter = rate.NewLimiter(rate.Limit(24), 1) // 24 requests per second

const (
	infuraURL           = "https://flashy-intensive-glade.base-mainnet.discover.quiknode.pro/api-key/"
	pairCreatedTopic    = "0x783cca1c0412dd0d695e784568c96da2e9c22ff989357a2e8b1d9b2b4e6b7118" // Replace with the actual topic for PairCreated
	poolCreatedTopic    = "0x0d3648bd0f6ba80134a33ba9275ac585d9d315f0ad8355cddefde31afa28d0e9" // Replace with the actual topic for PoolCreated
	NewPoolCreatedTopic = "0xf04da67755adf58739649e2fb9949a6328518141b7ac9e44aa10320688b04900"
	pairCreatedTopic_2  = "0xc4805696c66d7cf352fc1d6bb633ad5ee82f6cb577c453024b6e0eb8306c6fc9"
	pairCreatedTopic_3  = "0x2128d88d14c80cb081c1252a5acff7a264671bf199ce226b53788fb26065005e"
)

var db *sql.DB

func initDB() {
	// Set up the database connection.

	connStr := "user=emmett dbname=cryptoarch sslmode=disable password=password"
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}
}

func insertPair(pairAddress, token0Address, token1Address, deployerAddress, factoryAddress string) {
	query := `
        INSERT INTO pairs (pair_address, token0_address, token1_address, deployer_address, factory_address)
        VALUES ($1, $2, $3, $4, $5)
    `
	_, err := db.Exec(query, pairAddress, token0Address, token1Address, deployerAddress, factoryAddress)
	if err != nil {
		log.Printf("Failed to insert pair: %v", err)
	}
}

func main() {
	initDB() // Initialize the database

	log.Println("Starting script...")

	rpcClient, err := rpc.Dial(infuraURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	client := ethclient.NewClient(rpcClient)

	startBlock := big.NewInt(0)     // Replace with the actual starting block number
	endBlock := big.NewInt(4973333) // Replace with the actual ending block number or use nil for the latest block

	pageSize := big.NewInt(10000) // Set page size to 10,000 blocks

	for currentPage := new(big.Int).Set(startBlock); currentPage.Cmp(endBlock) < 0; currentPage.Add(currentPage, pageSize) {
		// Calculate fromBlock and toBlock for the current page
		fromBlock := new(big.Int).Set(currentPage)
		toBlock := new(big.Int).Add(new(big.Int).Set(currentPage), pageSize)
		if toBlock.Cmp(endBlock) > 0 {
			toBlock = endBlock
		}

		log.Printf("Processing blocks %s to %s...\n", fromBlock.String(), toBlock.String())

		query := ethereum.FilterQuery{
			FromBlock: fromBlock,
			ToBlock:   toBlock,
			Topics: [][]common.Hash{
				{
					common.HexToHash(pairCreatedTopic),
					common.HexToHash(poolCreatedTopic),
					common.HexToHash(NewPoolCreatedTopic),
					common.HexToHash(pairCreatedTopic_2),
					common.HexToHash(pairCreatedTopic_3),
				},
			},
		}

		log.Printf("Filtering logs for block range %s to %s", fromBlock.String(), toBlock.String())

		// Wait for permission from the rate limiter
		if err := limiter.Wait(context.TODO()); err != nil {
			log.Fatalf("Rate limiter error: %v", err)
		}

		logs, err := client.FilterLogs(context.Background(), query)
		if err != nil {
			log.Fatalf("Failed to filter logs: %v", err)
		}

		log.Printf("Found %d logs", len(logs))

		for _, vLog := range logs {
			deployerAddress := vLog.Address // This captures the deployer address from the log

			token0 := common.BytesToAddress(vLog.Topics[1].Bytes()[12:])
			token1 := common.BytesToAddress(vLog.Topics[2].Bytes()[12:])
			factoryAddress := vLog.Address.Hex() // This captures the factory address from the log address

			// Determine the event type based on the first topic
			switch vLog.Topics[0].Hex() {
			case pairCreatedTopic: // PoolCreated event
				// Extract the pool address from the last 20 bytes of the first 32-byte segment of the data field
				poolAddress := common.BytesToAddress(vLog.Data[32:64]).Hex()
				log.Printf("New Pool Created: %s, Tokens: %s, %s\n, Deployer: %s, Factory Address: %s", poolAddress, token0.Hex(), token1.Hex(), deployerAddress.Hex(), factoryAddress)

				// Update the call to insertPair to include the deployer and factory address
				insertPair(poolAddress, token0.Hex(), token1.Hex(), deployerAddress.Hex(), factoryAddress)

			case poolCreatedTopic: // PairCreated event
				token0 := common.BytesToAddress(vLog.Topics[1].Bytes()[12:])
				token1 := common.BytesToAddress(vLog.Topics[2].Bytes()[12:])
				factoryAddress := vLog.Address.Hex() // This captures the factory address from the log address

				// The pair address is in the data field; extract the address from the data field.
				if len(vLog.Data) < 32 {
					log.Printf("Insufficient data in log entry: %+v\n", vLog)
					continue // Skip this log entry
				}
				pairAddress := common.BytesToAddress(vLog.Data[12:32]).Hex() // Extracting the pool address from the data field

				log.Printf("New Pair Created: %s, Tokens: %s, %s\n, Deployer: %s, Factory Address: %s", pairAddress, token0.Hex(), token1.Hex(), deployerAddress.Hex(), factoryAddress)

				// Update the call to insertPair to include the deployer and factory address
				insertPair(pairAddress, token0.Hex(), token1.Hex(), deployerAddress.Hex(), factoryAddress)

			case NewPoolCreatedTopic: // NewPool event
				tokenX := common.BytesToAddress(vLog.Topics[1].Bytes()[12:])
				tokenY := common.BytesToAddress(vLog.Topics[2].Bytes()[12:])
				factoryAddress := vLog.Address.Hex() // This captures the factory address from the log address

				// The pool address is in the data field; extract the address from the data field.
				if len(vLog.Data) < 32 {
					log.Printf("Insufficient data in log entry: %+v\n", vLog)
					continue // Skip this log entry
				}
				poolAddress := common.BytesToAddress(vLog.Data[44:64]).Hex() // Extracting the pool address from the data field

				log.Printf("New Pool Created: %s, Tokens: %s, %s\n, Deployer: %s, Factory Address: %s", poolAddress, tokenX.Hex(), tokenY.Hex(), deployerAddress.Hex(), factoryAddress)

				// Update the call to insertPair to include the deployer and factory address
				insertPair(poolAddress, tokenX.Hex(), tokenY.Hex(), deployerAddress.Hex(), factoryAddress)

			case pairCreatedTopic_2: // PairCreated event for the specified contract
				token0 := common.BytesToAddress(vLog.Topics[1].Bytes()[12:])
				token1 := common.BytesToAddress(vLog.Topics[2].Bytes()[12:])
				factoryAddress := vLog.Address.Hex() // This captures the factory address from the log address

				// The pair address and stable flag are in the data field; check the data field length first
				if len(vLog.Data) < 64 {
					log.Printf("Insufficient data in log entry: %+v\n", vLog)
					continue // Skip this log entry
				}
				pairAddress := common.BytesToAddress(vLog.Data[44:64]).Hex()

				log.Printf("New Pair Created: %s, Tokens: %s, %s\n, Deployer: %s, Factory Address: %s", pairAddress, token0.Hex(), token1.Hex(), deployerAddress.Hex(), factoryAddress)

				// Update the call to insertPair to include the deployer and factory address, and possibly update the function to handle the stable flag
				insertPair(pairAddress, token0.Hex(), token1.Hex(), deployerAddress.Hex(), factoryAddress)

			case pairCreatedTopic_3: // PoolCreated event for the specified contract
				token0 := common.BytesToAddress(vLog.Topics[1].Bytes()[12:])
				token1 := common.BytesToAddress(vLog.Topics[2].Bytes()[12:])
				factoryAddress := vLog.Address.Hex() // This captures the factory address from the log address

				// The stable flag is in the third topic
				stable := vLog.Topics[3].Big().Uint64() != 0

				// Check the length of the data field to ensure it contains at least one 32-byte parameter
				if len(vLog.Data) < 32 {
					log.Printf("Insufficient data in log entry: %+v\n", vLog)
					continue // Skip this log entry
				}
				poolAddress := common.BytesToAddress(vLog.Data[12:32]).Hex() // Extracting the pool address from the data field

				log.Printf("New Pool Created: %s, Tokens: %s, %s\n, Stable: %v, Deployer: %s, Factory Address: %s", poolAddress, token0.Hex(), token1.Hex(), stable, deployerAddress.Hex(), factoryAddress)

				// Update the call to insertPair to include the deployer and factory address, and possibly update the function to handle the stable flag
				insertPair(poolAddress, token0.Hex(), token1.Hex(), deployerAddress.Hex(), factoryAddress)
			}
		}
	}

	log.Println("Script completed.")
}

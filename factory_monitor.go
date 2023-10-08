package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	_ "github.com/lib/pq"
	"golang.org/x/time/rate"
)

var limiter = rate.NewLimiter(rate.Limit(24), 1) // 25 requests per second

type Config struct {
	InfuraURL string `json:"infuraURL"`
	Pass      string `json:"db_password"`
}

const (
	infuraURL  = "https://flashy-intensive-glade.base-mainnet.discover.quiknode.pro/api-key/"
	factoryAbi = `[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"token0","type":"address"},{"indexed":true,"internalType":"address","name":"token1","type":"address"},{"indexed":false,"internalType":"address","name":"pair","type":"address"}],"name":"PairCreated","type":"event"}]`
)

type Factory struct {
	InternalDeployer string `json:"internal_deployer"`
	EntityID         string `json:"entity_id"`
}

type PairCreatedEvent struct {
	Stable bool // new field for the 'stable' boolean
	Pair   common.Address
	From   common.Address // New field to capture the deployer address
}

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

	eventABI, err := abi.JSON(strings.NewReader(factoryAbi))
	if err != nil {
		log.Fatalf("Failed to parse ABI: %v", err)
	}

	log.Println("Loading factories from factories.json...")
	data, err := os.ReadFile("factories.json")
	if err != nil {
		log.Fatalf("Failed to read factories.json: %v", err)
	}

	var factories []Factory
	err = json.Unmarshal(data, &factories)
	if err != nil {
		log.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	startBlock := big.NewInt(1500000) // Replace with the actual starting block number
	endBlock := big.NewInt(4000000)   // Replace with the actual ending block number or use nil for the latest block

	pageSize := big.NewInt(10000) // Set page size to 10,000 blocks

	for currentPage := new(big.Int).Set(startBlock); currentPage.Cmp(endBlock) < 0; currentPage.Add(currentPage, pageSize) {
		// Calculate fromBlock and toBlock for the current page
		fromBlock := new(big.Int).Set(currentPage)
		toBlock := new(big.Int).Add(new(big.Int).Set(currentPage), pageSize)
		if toBlock.Cmp(endBlock) > 0 {
			toBlock = endBlock
		}

		log.Printf("Processing blocks %s to %s...\n", fromBlock.String(), toBlock.String())

		for _, factory := range factories {
			log.Printf("Processing factory: %s", factory.InternalDeployer)

			factoryAddress := common.HexToAddress(factory.InternalDeployer)

			query := ethereum.FilterQuery{
				FromBlock: fromBlock,
				ToBlock:   toBlock,
				Addresses: []common.Address{factoryAddress},
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

			log.Printf("Found %d logs for factory %s", len(logs), factory.InternalDeployer)

			// ... rest of the processing code ...
			for _, vLog := range logs {
				// Check that there are enough topics.
				deployerAddress := vLog.Address            // This captures the deployer address from the log
				factoryAddress := factory.InternalDeployer // This captures the factory address from the current factory

				if len(vLog.Topics) < 3 {
					log.Println("Unexpected number of topics, skipping log entry.")
					continue
				}

				if len(vLog.Data) == 0 {
					log.Printf("Empty data field in log entry: %+v\n", vLog)
					continue // Skip this log entry
				}

				// The first topic is the event signature, the following topics are the indexed parameters.
				// Trim leading zeros to get the correct Ethereum address.
				token0 := common.BytesToAddress(vLog.Topics[1].Bytes()[12:])
				token1 := common.BytesToAddress(vLog.Topics[2].Bytes()[12:])

				// Now, unpack the non-indexed parameters from the data field.
				var event PairCreatedEvent
				err := eventABI.UnpackIntoInterface(&event, "PairCreated", vLog.Data)
				if err != nil {
					log.Fatalf("Failed to unpack event data: %v", err)
				}

				log.Printf("New Pair Created: %s, Tokens: %s, %s\n, Deployer: %s, Factory Address: %s", event.Pair.Hex(), token0.Hex(), token1.Hex(), deployerAddress.Hex(), factoryAddress)

				// Update the call to insertPair to include the deployer and factory address
				insertPair(event.Pair.Hex(), token0.Hex(), token1.Hex(), deployerAddress.Hex(), factoryAddress)
			}
		}
	}

	log.Println("Script completed.")
}

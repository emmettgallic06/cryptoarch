package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/big"
	"net/http"
	"strconv"

	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	infuraURL       = "https://flashy-intensive-glade.base-mainnet.discover.quiknode.pro/fc937e6a917d8493f86719ba041482277cfd3e26/"
	liquidityPoolSC = "0xaD5Cc4c2a6625151F9063D3373526abad2Ab846c"
	WETHAddress     = "0x4200000000000000000000000000000000000006"
)

// functionSelector computes the first 4 bytes of the keccak256 hash
// of the provided function signature, which represents its Method ID.
func functionSelector(funcSignature string) string {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(funcSignature))
	return "0x" + fmt.Sprintf("%x", hasher.Sum(nil)[:4])
}

// ethCall executes a call to the Ethereum network using the provided data.
func ethCall(client *rpc.Client, to common.Address, data string) (string, error) {
	args := map[string]interface{}{
		"to":   to.Hex(),
		"data": data,
	}

	var res string
	err := client.Call(&res, "eth_call", args, "latest")
	return res, err
}

func main() {
	client, err := rpc.Dial(infuraURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	// Fetch token addresses
	token0Address := callContract(client, functionSelector("token0()"))
	token1Address := callContract(client, functionSelector("token1()"))

	// Make WETH the base token, reorder if necessary
	baseTokenAddress, quoteTokenAddress := orderBaseQuoteTokens(token0Address, token1Address)

	// Fetch symbols
	baseTokenSymbol := getSymbol(client, baseTokenAddress)
	quoteTokenSymbol := getSymbol(client, quoteTokenAddress)

	// Fetch balances using the reordered base and quote addresses
	baseTokenBalance := getBalanceWithAdjustments(client, baseTokenAddress)
	quoteTokenBalance := getBalanceWithAdjustments(client, quoteTokenAddress)

	price := computePrice(baseTokenBalance, quoteTokenBalance)

	fmt.Printf("%s Balance: %s\n", baseTokenSymbol, baseTokenBalance.Text('f', 10))
	fmt.Printf("%s Balance: %s\n", quoteTokenSymbol, quoteTokenBalance.Text('f', 10))
	fmt.Printf("Price (%s/%s): %s\n", quoteTokenSymbol, baseTokenSymbol, price.Text('f', 10))

	// Compute the token price in USD if WETH is involved
	tokenPriceWETH, _ := price.Float64()
	wethPriceUSD := getWETHPriceUSD()

	tokenPriceUSD := tokenPriceWETH * wethPriceUSD

	fmt.Printf("WETH Price: %.2f\n", wethPriceUSD)
	fmt.Printf("Price (%s in USD): %.8f\n", quoteTokenSymbol, tokenPriceUSD)
}

// orderBaseQuoteTokens checks and reorders the tokens to ensure WETH is the base token.
func orderBaseQuoteTokens(token0, token1 common.Address) (base, quote common.Address) {
	if token0.Hex() == WETHAddress {
		return token0, token1
	}
	return token1, token0
}

// callContract fetches the address at the end of the execution result of the provided data.
func callContract(client *rpc.Client, data string) common.Address {
	res, err := ethCall(client, common.HexToAddress(liquidityPoolSC), data)
	if err != nil {
		log.Fatalf("Failed to call contract: %v", err)
	}
	return common.HexToAddress(res[len(res)-40:])
}

// getSymbol fetches the symbol of a token.
func getSymbol(client *rpc.Client, tokenAddress common.Address) string {
	data := functionSelector("symbol()")
	res, err := ethCall(client, tokenAddress, data)
	if err != nil {
		log.Fatalf("Failed to get symbol: %v", err)
	}

	// The returned value is a hex encoded string, so we need to convert it to ASCII
	bytes, err := hex.DecodeString(res[2:])
	if err != nil {
		log.Fatalf("Failed to decode symbol: %v", err)
	}
	return string(bytes)
}

// getBalance fetches the balance of an account for a specific token.
func getBalance(client *rpc.Client, tokenAddress, accountAddress common.Address) *big.Int {
	data := functionSelector("balanceOf(address)") + hex.EncodeToString(common.LeftPadBytes(accountAddress.Bytes(), 32))
	res, err := ethCall(client, tokenAddress, data)
	if err != nil {
		log.Fatalf("Failed to get balance: %v", err)
	}

	balance := new(big.Int)
	balance.SetString(res[2:], 16)
	return balance
}

// getDecimals fetches the decimal precision of a token.
func getDecimals(client *rpc.Client, tokenAddress common.Address) int {
	data := functionSelector("decimals()")
	res, err := ethCall(client, tokenAddress, data)
	if err != nil {
		log.Fatalf("Failed to get decimals: %v", err)
	}

	decimals, _ := strconv.ParseInt(res[2:], 16, 64)
	return int(decimals)
}

// adjustBalance adjusts the raw balance of a token based on its decimal precision.
func adjustBalance(balance *big.Int, decimals int) *big.Float {
	multiplier := new(big.Float).SetFloat64(math.Pow10(decimals))
	adjustedBalance := new(big.Float).SetInt(balance)
	return new(big.Float).Quo(adjustedBalance, multiplier)
}

// computePrice calculates the price of token1 in terms of token0.
func computePrice(balance0, balance1 *big.Float) *big.Float {
	return new(big.Float).Quo(balance0, balance1)
}

// getBalanceWithAdjustments fetches the balance and adjusts it based on the decimals
func getBalanceWithAdjustments(client *rpc.Client, tokenAddress common.Address) *big.Float {
	liquidityAddress := common.HexToAddress(liquidityPoolSC)
	balanceRaw := getBalance(client, tokenAddress, liquidityAddress)
	decimals := getDecimals(client, tokenAddress)
	return adjustBalance(balanceRaw, decimals)
}

func getWETHPriceUSD() float64 {
	resp, err := http.Get("https://api.coingecko.com/api/v3/simple/price?ids=weth&vs_currencies=usd")
	if err != nil {
		log.Fatalf("Failed to fetch WETH price: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatalf("Failed to decode response: %v", err)
	}

	return result["weth"]["usd"]
}

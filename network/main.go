package network

import (
	"crypto/sha256"
	"os"

	"github.com/ecnepsnai/discord"
	"github.com/shopspring/decimal"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
)

var kTempAccountSalt string = "j4rkTZQ2mLk3NAhK"

func GetBlockchainNetworkPassPhrase() string {
	return os.Getenv("BLOCKCHAIN_NETWORK_PASSPHRASE")
}

func GetBlockchainBaseReserve() decimal.Decimal {
	val, err := decimal.NewFromString(os.Getenv("BLOCKCHAIN_BASE_RESERVE"))

	if err != nil {
		return decimal.NewFromInt(1)
	}

	return val

}

func GetBlockchainClient() *horizonclient.Client {
	var client *horizonclient.Client = horizonclient.DefaultPublicNetClient
	client.HorizonURL = os.Getenv("EXPANSION_URL")
	return client
}

func TempAccountKeypair(publicKey string) (*keypair.Full, error) {

	mnemonic := os.Getenv("MNEMONIC_TEMP_ACCOUNTS")

	h := sha256.New()
	h.Write([]byte(kTempAccountSalt))
	h.Write([]byte(mnemonic))
	h.Write([]byte(publicKey))

	hashed := h.Sum(nil)

	var rawSeed [32]byte
	copy(rawSeed[:], hashed[0:32])

	return keypair.FromRawSeed([32]byte(rawSeed))

}

func LogDiscordError(msg string) {
	discord.WebhookURL = "https://discord.com/api/webhooks/865931042795290636/jObHzZWdnbhX1jomOSZQX8Ip5AXLArh87PI4-ZQ8u6ssnRbZuVdY_iPxz5qoWkHUlZwS"
	if len(os.Getenv("500_ERROR_WEBHOOK")) > 50 {
		discord.WebhookURL = os.Getenv("500_ERROR_WEBHOOK")
	}
	discord.Say(msg)
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"bantu-monitor/db"
	"bantu-monitor/network"
	root "bantu-monitor/root/controllers"

	"github.com/coreos/pkg/flagutil"
	"github.com/dghubble/oauth1"
	"github.com/drswork/go-twitter/twitter"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon/operations"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gorm.io/gorm"
)

type Lastcursor struct {
	ID     int
	Cursor string
}
type Asset struct {
	Code      string
	Issuer    string
	MinAmount decimal.Decimal
}

var knownWallets map[string]string
var trackedAssets map[string]Asset
var client *twitter.Client
var tc int

func init() {
	errEnv := godotenv.Load()
	if errEnv != nil {
		path, _ := os.Getwd()
		log.Printf("could not find or load any .env file from %v...skipping...\n", path)
	}

}
func main() {
	database, err := db.OpenSqliteDB()

	if err != nil {
		log.Fatal(err)
	}

	err = database.AutoMigrate(&Lastcursor{})
	if err != nil {
		log.Fatal(err)
	}

	// get known wallets(BITMART:GDBPIYT....SDF,FMFW:GDB...SDF)
	knownWallets = make(map[string]string, 0)

	//CODE:ISSUER:MIN_AMOUNT,CODE:ISSUER:MIN_AMOUNT
	trackedAssets = make(map[string]Asset, 0)
	listedWallets := strings.Split(strings.ReplaceAll(os.Getenv("KNOWN_WALLETS"), " ", ""), ",")
	for _, w := range listedWallets {
		k := strings.Split(w, ":")
		knownWallets[k[1]] = k[0]
	}
	//add native token
	trackedAssets[":"] = Asset{
		Code:      "",
		Issuer:    "",
		MinAmount: decimal.RequireFromString(os.Getenv("MIN_AMOUNT")),
	}
	log.Printf("tracking....%+v\n", Asset{
		Code:      "XBN",
		Issuer:    "native",
		MinAmount: decimal.RequireFromString(os.Getenv("MIN_AMOUNT")),
	})
	listedAssets := strings.Split(strings.ReplaceAll(os.Getenv("TRACKED_ASSETS"), " ", ""), ",")
	for _, w := range listedAssets {
		k := strings.Split(w, ":")
		asset := Asset{
			Code:      k[0],
			Issuer:    k[1],
			MinAmount: decimal.RequireFromString(k[2]),
		}
		trackedAssets[k[0]+":"+k[1]] = asset
		log.Printf("tracking....%+v\n", asset)

	}
	flags := flag.NewFlagSet("user-auth", flag.ExitOnError)
	consumerKey := flags.String("consumer-key", "", "Twitter Consumer Key")
	consumerSecret := flags.String("consumer-secret", "", "Twitter Consumer Secret")
	accessToken := flags.String("access-token", "", "Twitter Access Token")
	accessSecret := flags.String("access-secret", "", "Twitter Access Secret")
	flags.Parse(os.Args[1:])
	flagutil.SetFlagsFromEnv(flags, "TWITTER")

	if *consumerKey == "" || *consumerSecret == "" || *accessToken == "" || *accessSecret == "" {
		log.Fatal("Consumer key/secret and Access token/secret required")
	}
	// log.Println(*consumerKey)
	// log.Println(*consumerSecret)
	// log.Println(*accessToken)
	// log.Println(*accessSecret)

	config := oauth1.NewConfig(*consumerKey, *consumerSecret)
	token := oauth1.NewToken(*accessToken, *accessSecret)
	// OAuth1 http.Client will automatically authorize Requests
	httpClient := config.Client(oauth1.NoContext, token)

	// Twitter client
	client = twitter.NewClient(httpClient)

	go func() {
		for {
			MonitorStream(database)

			time.Sleep(5 * time.Second)
			log.Println("restarting stream..........")
		}

	}()
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	var router *gin.Engine = gin.Default()

	root.Init(router, trackedAssets)
	log.Println("##root services initialized##")

	//run app
	log.Println("##service started##")
	// router.Run(":" + os.Getenv("PORT"))

	if len(os.Getenv("PORT")) == 0 {
		log.Println(router.Run(":80"))
	} else {
		log.Println(router.Run(":" + os.Getenv("PORT")))
	}

}

func MonitorStream(db *gorm.DB) {
	client := network.GetBlockchainClient()
	workerChan := make(chan operations.Operation, 200000)
	lastCursor := GetLastCursor(db)
	var opsRequest horizonclient.OperationRequest
	if len(lastCursor) > 0 {
		log.Printf("[MonitorStream] Starting monitoring from cursor[%v]\n", lastCursor)
		opsRequest = horizonclient.OperationRequest{
			Cursor: lastCursor,
			Order:  horizonclient.OrderAsc,
			Join:   "transactions",
		}
	} else {
		log.Println("[MonitorStream] Starting monitoring from genesis for blockchain")

		opsRequest = horizonclient.OperationRequest{
			Cursor: "0",
			Order:  horizonclient.OrderAsc,
			Join:   "transactions",
		}
	}
	worker := func() {
		for {
			o := <-workerChan
			ProcessOperation(o, db)
			tc++
			SaveLastCursor(o.PagingToken(), db)
		}

	}
	{
		//start two workers
		go worker()
		// go worker()

	}

	operationsStreamHandler := func(o operations.Operation) {
		//send to worker channel
		workerChan <- o
	}

	ctx, cancel := context.WithCancel(context.Background())

	streamOperations := func() {

		err := client.StreamPayments(ctx, opsRequest, operationsStreamHandler)
		if err != nil {
			log.Printf("[MonitorStream] streamerr:[%v]", err)
			cancel()
		}

	}

	//Start stream
	streamOperations()
	log.Println("#####...Ending streaming operation")
	//close channels
	// close(workerChan)

}

func ProcessOperation(o operations.Operation, db *gorm.DB) {
	// minAmount := decimal.RequireFromString(os.Getenv("MIN_AMOUNT"))
	// maxAmount := decimal.RequireFromString(os.Getenv("MAX_AMOUNT"))
	// var maxPTSkip int
	maxPTSkip := 500000
	if o.GetType() == "payment" {
		pmt := interface{}(o).(operations.Payment)
		if tc > maxPTSkip {
			log.Println("CURSOR....", pmt.PT)
			tc = 0
		}

		as, ok := trackedAssets[pmt.Code+":"+pmt.Issuer]
		if !ok {
			return
		}
		destAssetCode := "XBN"
		if len(pmt.Code) > 0 {
			destAssetCode = pmt.Code
		}
		if decimal.RequireFromString(pmt.Amount).GreaterThanOrEqual(as.MinAmount) {
			SendTwitterMessage(pmt.Amount, pmt.From, pmt.To, "", destAssetCode, "", pmt.TransactionHash, false)

		}

	}
	if o.GetType() == "create_account" {
		pmt := interface{}(o).(operations.CreateAccount)
		if tc > maxPTSkip {
			log.Println("CURSOR....", pmt.PT)
			tc = 0
		}
		log.Println("CURSOR....", pmt.PT)
		destAssetCode := "XBN"
		as, ok := trackedAssets[":"]
		if !ok {
			return
		}
		if decimal.RequireFromString(pmt.StartingBalance).GreaterThanOrEqual(as.MinAmount) {
			SendTwitterMessage(pmt.StartingBalance, pmt.Funder, pmt.Account, "", destAssetCode, "", pmt.TransactionHash, false)

		}
	}

	if o.GetType() == "path_payment_strict_send" {
		// log.Println("SWAP....")
		pmt := interface{}(o).(operations.PathPaymentStrictSend)
		//send out to be saved to db
		// assetCode := "XBN"
		swapFrom := "XBN"
		swapTo := "XBN"
		asdest, okdest := trackedAssets[pmt.Code+":"+pmt.Issuer]
		assource, oksource := trackedAssets[pmt.SourceAssetCode+":"+pmt.SourceAssetIssuer]
		if !oksource && !okdest {
			return
		}
		if len(pmt.SourceAssetCode) > 0 {
			swapFrom = pmt.SourceAssetCode
		}
		if len(pmt.Code) > 0 {
			swapTo = pmt.Code
		}
		doAlt := true
		if oksource {
			if tc > maxPTSkip {
				log.Println("SWAP....", pmt.SourceAmount, pmt.SourceAssetCode, "Cursor:", pmt.PT)

				tc = 0
			}
			if decimal.RequireFromString(pmt.SourceAmount).GreaterThanOrEqual(assource.MinAmount) {
				SendTwitterMessage(pmt.Amount, pmt.From, pmt.To, swapFrom, swapTo, pmt.SourceAmount, pmt.TransactionHash, true)
				doAlt = false
			}
		} else if okdest && doAlt {
			if tc > maxPTSkip {
				log.Println("SWAP....", pmt.Amount, pmt.Code, "Cursor:", pmt.PT)

				tc = 0
			}
			if decimal.RequireFromString(pmt.Amount).GreaterThanOrEqual(asdest.MinAmount) {
				SendTwitterMessage(pmt.Amount, pmt.From, pmt.To, swapFrom, swapTo, pmt.SourceAmount, pmt.TransactionHash, true)

			}
		}

	}

	if o.GetType() == "path_payment" {

		pmt := interface{}(o).(operations.PathPayment)
		//send out to be saved to db
		// assetCode := "XBN"
		swapFrom := "XBN"
		swapTo := "XBN"
		asdest, okdest := trackedAssets[pmt.Code+":"+pmt.Issuer]
		assource, oksource := trackedAssets[pmt.SourceAssetCode+":"+pmt.SourceAssetIssuer]
		if !oksource && !okdest {
			return
		}
		if len(pmt.SourceAssetCode) > 0 {
			swapFrom = pmt.SourceAssetCode
		}
		if len(pmt.Code) > 0 {
			swapTo = pmt.Code
		}
		doAlt := true
		if oksource {
			if tc > maxPTSkip {
				log.Println("SWAP....", pmt.SourceAmount, pmt.SourceAssetCode, "Cursor:", pmt.PT)

				tc = 0
			}
			if decimal.RequireFromString(pmt.SourceAmount).GreaterThanOrEqual(assource.MinAmount) {
				SendTwitterMessage(pmt.Amount, pmt.From, pmt.To, swapFrom, swapTo, pmt.SourceAmount, pmt.TransactionHash, true)
				doAlt = false
			}
		} else if okdest && doAlt {

			if tc > maxPTSkip {
				log.Println("SWAP....", pmt.Amount, pmt.Code, "Cursor:", pmt.PT)

				tc = 0
			}
			if decimal.RequireFromString(pmt.Amount).GreaterThanOrEqual(asdest.MinAmount) {
				SendTwitterMessage(pmt.Amount, pmt.From, pmt.To, swapFrom, swapTo, pmt.SourceAmount, pmt.TransactionHash, true)

			}
		}

	}

}

func GetLastCursor(db *gorm.DB) (lastCursor string) {
	var envCusor string
	if len(os.Getenv("LAST_CURSOR")) > 0 {
		envCusor = os.Getenv("LAST_CURSOR")
	} else {
		envCusor = "0"
	}
	var lc Lastcursor

	e := db.First(&lc).Error
	if e == nil {
		if decimal.RequireFromString(envCusor).GreaterThan(decimal.RequireFromString(lc.Cursor)) {
			return envCusor
		}
		return lc.Cursor
	}
	return envCusor
}
func SaveLastCursor(lastCursor string, db *gorm.DB) {
	var lc Lastcursor
	e := db.First(&lc).Error
	if e == nil {
		lc.Cursor = lastCursor
		e := db.Save(&lc).Error
		if e != nil {
			log.Fatalf("error saving last cursor %v\n", e)

		}
		return
	}
	lc = Lastcursor{
		Cursor: lastCursor,
	}
	e = db.Create(&lc).Error
	if e != nil {
		log.Fatalf("error creating last cursor %v\n", e)

	}

}

func SendTwitterMessage(destAmount, fromWallet, toWallet, swapFromAsset, swapToAsset, swapFromAmount, tx string, swap bool) {
	var msg string
	explorer := os.Getenv("EXPLORER_URL") + tx
	if v, ok := knownWallets[fromWallet]; ok {
		fromWallet = "#" + v
	} else {
		bfrom := []byte(fromWallet)
		fromWallet = fmt.Sprintf("%s...%s", bfrom[0:3], bfrom[52:])
	}
	if v, ok := knownWallets[toWallet]; ok {
		toWallet = "#" + v
	} else {
		bto := []byte(toWallet)
		toWallet = fmt.Sprintf("%s...%s", bto[0:3], bto[52:])
	}
	p := message.NewPrinter(language.English)
	a, _ := decimal.RequireFromString(destAmount).Float64()
	DestAmountWithCommaThousandSep := p.Sprintf("%f", a)

	if swap {
		d, _ := decimal.RequireFromString(swapFromAmount).Float64()
		sourceWithCommaThousandSep := p.Sprintf("%f", d)
		msg = fmt.Sprintf("🚨🚨🚨 %s swapped %s $%s to %s $%s on wallet %s. @bantublockchain : %s", fromWallet, sourceWithCommaThousandSep, swapFromAsset, DestAmountWithCommaThousandSep, swapToAsset, toWallet, explorer)

		// if strings.Contains(toWallet, "...") {
		// 	msg = fmt.Sprintf("🚨🚨🚨 %s swapped %s $%s to %s $%s on wallet %s. @bantublockchain : %s", fromWallet, sourceWithCommaThousandSep, swapFromAsset, DestAmountWithCommaThousandSep, swapToAsset, toWallet, explorer)

		// } else {
		// 	msg = fmt.Sprintf("🚨🚨🚨 %s swapped %s $%s to %s $%s on wallet #%s. @bantublockchain : %s", fromWallet, sourceWithCommaThousandSep, swapFromAsset, DestAmountWithCommaThousandSep, swapToAsset, toWallet, explorer)

		// }
	} else {
		msg = fmt.Sprintf("🚨🚨🚨  %s $%s transferred from %s wallet to #%s. @bantublockchain : %s", DestAmountWithCommaThousandSep, swapToAsset, fromWallet, toWallet, explorer)

		// if strings.Contains(toWallet, "...") {
		// 	msg = fmt.Sprintf("🚨🚨🚨  %s $%s transferred from %s wallet to %s. @bantublockchain : %s", DestAmountWithCommaThousandSep, swapToAsset, fromWallet, toWallet, explorer)

		// } else {
		// 	msg = fmt.Sprintf("🚨🚨🚨  %s $%s transferred from %s wallet to #%s. @bantublockchain : %s", DestAmountWithCommaThousandSep, swapToAsset, fromWallet, toWallet, explorer)

		// }

	}
	TwitterStatusUpdate(msg)
}
func SendTelegramMessage() {

}
func TwitterStatusUpdate(status string) {

	update, rp, err := client.Statuses.Update(status,
		&twitter.StatusUpdateParams{Status: status},
	)
	if err != nil {
		log.Println(err)
	}

	if rp.Status != "200 OK" {
		log.Println(rp.Status)
	}

	fmt.Printf("status update Show:\n%+v, %v\n", update.ID, err)

}

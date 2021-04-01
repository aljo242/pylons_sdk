package inttest

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Pylons-tech/pylons_sdk/app"
	testing "github.com/Pylons-tech/pylons_sdk/cmd/evtesting"
	"github.com/Pylons-tech/pylons_sdk/x/pylons/msgs"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	log "github.com/sirupsen/logrus"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

// CLIOptions is a struct to manage pylonsd options
type CLIOptions struct {
	CustomNode   string
	RestEndpoint string
	MaxWaitBlock int64
	MaxBroadcast int
}

// CLIOpts is a variable to manage pylonsd options
var CLIOpts CLIOptions
var cliMux sync.Mutex

func init() {
	flag.StringVar(&CLIOpts.CustomNode, "node", "tcp://localhost:26657", "custom node url")
}

// GetMaxWaitBlock is a function to get configuration for maximum wait block, default 3
func GetMaxWaitBlock() int64 {
	if CLIOpts.MaxWaitBlock == 0 {
		return 3
	}
	return CLIOpts.MaxWaitBlock
}

// GetMaxBroadcastRetry is a function to get configuration for maximum retry for transactio broadcast
func GetMaxBroadcastRetry() int {
	if CLIOpts.MaxBroadcast == 0 {
		return 50
	}
	return CLIOpts.MaxBroadcast
}

// ReadFile is a utility function to read file
func ReadFile(fileURL string, t *testing.T) []byte {
	jsonFile, err := os.Open(fileURL)
	if err != nil {
		t.MustNil(err, "error reading file")
		return []byte{}
	}

	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)
	return byteValue
}

// GetAminoCdc is a utility function to get amino codec
func GetAminoCdc() *codec.LegacyAmino {
	return app.MakeEncodingConfig().Amino
}

// KeyringBackendSetup is a utility function to setup keyring backend for pylonsd command
func KeyringBackendSetup(args []string) []string {
	if len(args) == 0 {
		return args
	}
	newArgs := append(args, "--keyring-backend", "test")
	switch args[0] {
	case "keys":
		return newArgs
	case "tx":
		if args[1] == "sign" {
			return newArgs
		}
		if args[1] == "pylons" && args[2] == "create-account" {
			return newArgs
		}
		return args
	default:
		return args
	}
}

// NodeFlagSetup is a utility function to setup configured custom node
func NodeFlagSetup(args []string) []string {
	if len(CLIOpts.CustomNode) > 0 {
		if args[0] == "query" || args[0] == "tx" || args[0] == "status" {
			customNodes := strings.Split(CLIOpts.CustomNode, ",")
			randNodeIndex := rand.Intn(len(customNodes))
			randNode := customNodes[randNodeIndex]
			args = append(args, "--node", randNode)
		}
	}
	return args
}

// RunPylonsd is a function to run pylonsd
func RunPylonsd(args []string, stdinInput string) ([]byte, string, error) {
	args = NodeFlagSetup(args)
	args = KeyringBackendSetup(args)
	cliMux.Lock()
	cmd := exec.Command(path.Join(os.Getenv("GOPATH"), "/bin/pylonsd"), args...)
	cmd.Stdin = strings.NewReader(stdinInput)
	res, err := cmd.CombinedOutput()
	cliMux.Unlock()
	return res, fmt.Sprintf("\"pylonsd %s\" ==>\n%s\n", strings.Join(args, " "), string(res)), err
}

// GetAccountAddr is a function to get account address from key
func GetAccountAddr(account string, t *testing.T) string {
	addrBytes, logstr, err := RunPylonsd([]string{"keys", "show", account, "-a"}, "")
	addr := strings.Trim(string(addrBytes), "\n ")
	t.WithFields(testing.Fields{
		"account": account,
		"log":     logstr,
	}).MustNil(err, "error getting account address")
	return addr
}

// GetAccountInfoFromAddr is a function to get account information from address
func GetAccountInfoFromAddr(addr string, t *testing.T) authtypes.BaseAccount {
	var accInfo authtypes.BaseAccount
	accBytes, logstr, err := RunPylonsd([]string{"query", "account", addr}, "")
	t.WithFields(testing.Fields{
		"address": addr,
		"log":     logstr,
	}).MustNil(err, "error getting account info")
	if err != nil {
		return accInfo
	}
	err = GetAminoCdc().UnmarshalJSON(accBytes, &accInfo)
	t.WithFields(testing.Fields{
		"acc_bytes": string(accBytes),
	}).MustNil(err, "error decoding raw json")
	// t.WithFields(testing.Fields{
	// 	"account_info": accInfo,
	// }).Debug("debug log")
	return accInfo
}

// GetAccountInfoFromAddr is a function to get account information from address
func GetAccountBalanceFromAddr(addr string, t *testing.T) banktypes.Balance {
	var balance banktypes.Balance
	accBytes, logstr, err := RunPylonsd([]string{"query", "bank", "balances", addr}, "")
	t.WithFields(testing.Fields{
		"address": addr,
		"log":     logstr,
	}).MustNil(err, "error getting account balance")
	if err != nil {
		return balance
	}
	err = GetAminoCdc().UnmarshalJSON(accBytes, &balance)
	t.WithFields(testing.Fields{
		"acc_bytes": string(accBytes),
	}).MustNil(err, "error decoding raw json")
	// t.WithFields(testing.Fields{
	// 	"account_info": accInfo,
	// }).Debug("debug log")
	return balance
}

// GetAccountInfoFromName is a function to get account information from account key
func GetAccountInfoFromName(account string, t *testing.T) authtypes.BaseAccount {
	addr := GetAccountAddr(account, t)
	return GetAccountInfoFromAddr(addr, t)
}

// GetDaemonStatus is a function to get daemon status
func GetDaemonStatus() (*ctypes.ResultStatus, string, error) {
	var ds ctypes.ResultStatus

	dsBytes, logstr, err := RunPylonsd([]string{"status"}, "")

	if err != nil {
		return nil, logstr, err
	}
	err = GetAminoCdc().UnmarshalJSON(dsBytes, &ds)

	if err != nil {
		return nil, logstr, err
	}
	return &ds, logstr, nil
}

// WaitForNextBlock is a function to wait until next block
func WaitForNextBlock() error {
	return WaitForBlockInterval(1)
}

// WaitForBlockInterval is a function to wait until block heights to flow
func WaitForBlockInterval(interval int64) error {
	ds, _, err := GetDaemonStatus()
	if err != nil {
		return err // couldn't get daemon status.
	}
	currentBlock := ds.SyncInfo.LatestBlockHeight

	counter := int64(1)
	for counter < 300*interval {
		ds, _, err = GetDaemonStatus()
		if err != nil {
			return err
		}
		if ds.SyncInfo.LatestBlockHeight >= currentBlock+interval {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
		counter++
	}
	return errors.New("You are waiting too long time for interval")
}

// CleanFile is a function to remove file
func CleanFile(filePath string, t *testing.T) {
	err := os.Remove(filePath)
	if err != nil {
		t.WithFields(testing.Fields{
			"error":     err,
			"file_path": filePath,
		}).Error("error removing file")
	}
}

// AminoCodecFormatter format structs better by encoding in amino codec
func AminoCodecFormatter(param interface{}) string {
	cdc := GetAminoCdc()
	output, err := cdc.MarshalJSON(param)
	if err == nil {
		return string(output)
	}
	return fmt.Sprintf("%+v", param)
}

// GetLogFieldsFromMsgs fetch mandatory keys from msgs for debugging
func GetLogFieldsFromMsgs(txMsgs []sdk.Msg) log.Fields {
	fields := log.Fields{}
	for idx, msg := range txMsgs {
		ikeypref := fmt.Sprintf("tx_msg%d_", idx)
		if len(txMsgs) == 1 {
			ikeypref = "tx_msg_"
		}
		switch msg := msg.(type) {
		case *msgs.MsgCreateCookbook:
			fields[ikeypref+"type"] = "MsgCreateCookbook"
			fields[ikeypref+"cb_name"] = msg.Name
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgUpdateCookbook:
			fields[ikeypref+"type"] = "MsgUpdateCookbook"
			fields[ikeypref+"cb_ID"] = msg.ID
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgCreateRecipe:
			fields[ikeypref+"type"] = "MsgCreateRecipe"
			fields[ikeypref+"rcp_name"] = msg.Name
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgUpdateRecipe:
			fields[ikeypref+"type"] = "MsgUpdateRecipe"
			fields[ikeypref+"rcp_name"] = msg.Name
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgExecuteRecipe:
			fields[ikeypref+"type"] = "MsgExecuteRecipe"
			fields[ikeypref+"rcp_id"] = msg.RecipeID
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgEnableRecipe:
			fields[ikeypref+"type"] = "MsgEnableRecipe"
			fields[ikeypref+"rcp_id"] = msg.RecipeID
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgDisableRecipe:
			fields[ikeypref+"type"] = "MsgDisableRecipe"
			fields[ikeypref+"rcp_id"] = msg.RecipeID
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgCheckExecution:
			fields[ikeypref+"type"] = "MsgCheckExecution"
			fields[ikeypref+"exec_id"] = msg.ExecID
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgCreateTrade:
			fields[ikeypref+"type"] = "MsgCreateTrade"
			fields[ikeypref+"trade_info"] = msg.ExtraInfo
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgFulfillTrade:
			fields[ikeypref+"type"] = "MsgFulfillTrade"
			fields[ikeypref+"trade_id"] = msg.TradeID
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgFiatItem:
			fields[ikeypref+"type"] = "MsgFiatItem"
			fields[ikeypref+"sender"] = msg.Sender
		case *msgs.MsgUpdateItemString:
			fields[ikeypref+"type"] = "MsgUpdateItemString"
			fields[ikeypref+"item_id"] = msg.ItemID
			fields[ikeypref+"sender"] = msg.Sender
		}
	}
	return fields
}

// JSONFormatter format structs better by encoding in amino codec
func JSONFormatter(param interface{}) string {
	output, err := json.Marshal(param)
	if err == nil {
		return string(output)
	}
	return fmt.Sprintf("%+v;jsonMarshalErr=%s", param, err.Error())
}

// Exists check if element exist in an array
func Exists(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// GetTxHashFromLog returns txhash from long list of transaction log
func GetTxHashFromLog(result string) string {
	// use regexp to find txhash from cli command response
	re := regexp.MustCompile(`"txhash":.*"(.*)"`)
	caTxHashSearch := re.FindSubmatch([]byte(result))
	if len(caTxHashSearch) <= 1 {
		return ""
	}
	caTxHash := string(caTxHashSearch[1])
	return caTxHash
}

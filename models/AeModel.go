package models

import (
	"box/utils"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aeternity/aepp-sdk-go/account"
	"github.com/aeternity/aepp-sdk-go/aeternity"
	"github.com/aeternity/aepp-sdk-go/binary"
	"github.com/aeternity/aepp-sdk-go/config"
	"github.com/aeternity/aepp-sdk-go/naet"
	"github.com/aeternity/aepp-sdk-go/swagguard/node/models"
	"github.com/aeternity/aepp-sdk-go/transactions"
	"github.com/beego/cache"
	"github.com/tyler-smith/go-bip39"
	"io/ioutil"
	"math/big"
	"strconv"
	"time"
)

var NodeUrl = "https://node.aeasy.io"
var NodeUrlDebug = "https://debug.aeasy.io"
var CompilerUrl = "https://compiler.aeasy.io"

//var NodeUrl = "https://testnet.aeternity.io"
//var NodeUrlDebug = "https://testnet.aeternity.io"
//var CompilerUrl = "https://compiler.aeasy.io"
//ct_2bFV4kxtmUeKTF5Eb5STfnAx6UgxzBqTL1pYLF1GBhcmoQAhLf
var ABCLockContractV3 = "ct_2W5UZLXySwh5BYXnXocGrzZ4wvLJQza1UPTsvor2UrtvjoFfQt"

//var nodeURL = nodeURL
//根据助记词返回用户
func MnemonicAccount(mnemonic string) (*account.Account, error) {
	seed, err := account.ParseMnemonic(mnemonic)
	if err != nil {
		return nil, err
	}
	_, err = bip39.EntropyFromMnemonic(mnemonic)

	if err != nil {
		return nil, err
	}
	// Derive the subaccount m/44'/457'/3'/0'/1'
	key, err := account.DerivePathFromSeed(seed, 0, 0)
	if err != nil {
		return nil, err
	}

	// Deriving the aeternity Account from a BIP32 Key is a destructive process
	alice, err := account.BIP32KeyToAeKey(key)
	if err != nil {
		return nil, err
	}
	return alice, nil
}

//根据私钥返回用户
func SigningKeyHexStringAccount(signingKey string) (*account.Account, error) {
	acc, e := account.FromHexString(signingKey)
	return acc, e
}

//随机创建用户
func CreateAccount() (*account.Account, string) {
	mnemonic, signingKey, _ := CreateAccountUtils()
	acc, _ := account.FromHexString(signingKey)
	return acc, mnemonic
}

//随机创建用户
func CreateAccountUtils() (mnemonic string, signingKey string, address string) {

	//cerate mnemonic
	entropy, _ := bip39.NewEntropy(128)
	mne, _ := bip39.NewMnemonic(entropy)

	//mnemonic := "tail disagree oven fit state cube rule test economy claw nice stable"
	seed, _ := account.ParseMnemonic(mne)

	_, _ = bip39.EntropyFromMnemonic(mne)
	// Derive the subaccount m/44'/457'/3'/0'/1'
	key, _ := account.DerivePathFromSeed(seed, 0, 0)

	// Deriving the aeternity Account from a BIP32 Key is a destructive process
	alice, _ := account.BIP32KeyToAeKey(key)
	return mne, alice.SigningKeyToHexString(), alice.Address
}

//返回最新区块高度
func ApiBlocksTop() (height uint64) {
	client := naet.NewNode(NodeUrl, false)
	h, _ := client.GetHeight()
	return h
}

//地址信息返回用户信息
func ApiGetAccount(address string) (account *models.Account, e error) {
	client := naet.NewNode(NodeUrl, false)
	acc, e := client.GetAccount(address)
	return acc, e
}

//发起转账
func ApiSpend(account *account.Account, recipientId string, amount float64, data string) (*aeternity.TxReceipt, error) {
	//获取账户
	accountNet, e := ApiGetAccount(account.Address)
	if e != nil {
		return nil, e
	}
	//格式化账户的tokens
	tokens, err := strconv.ParseFloat(accountNet.Balance.String(), 64)
	if err == nil {

		//判断账户余额是否大于要转账的余额
		if tokens/1000000000000000000 >= amount {
			//获取节点信息
			node := naet.NewNode(NodeUrl, false)
			//生成ttl
			ttler := transactions.CreateTTLer(node)
			noncer := transactions.CreateNoncer(node)

			ttlNoncer := transactions.CreateTTLNoncer(ttler, noncer)
			//生成转账tx
			spendTx, err := transactions.NewSpendTx(account.Address, recipientId, utils.GetRealAebalanceBigInt(amount), []byte(data), ttlNoncer)

			if err != nil {
				return nil, err
			}
			//广播转账信息
			hash, err := aeternity.SignBroadcast(spendTx, account, node, "ae_mainnet")

			//err = aeternity.WaitSynchronous(hash, config.Client.WaitBlocks, node)

			if err != nil {
				return nil, err
			}
			return hash, err
		} else {
			return nil, errors.New("tokens number insufficient")
		}
	} else {
		return nil, err
	}
}

type CallInfoResult struct {
	CallInfo CallInfo `json:"call_info"`
	Reason   string   `json:"reason"`
}
type CallInfo struct {
	ReturnType  string `json:"return_type"`
	ReturnValue string `json:"return_value"`
}

//正常调用合约
func CallContractFunction(address string, ctID string, function string, args []string, amount float64) (tx *transactions.ContractCallTx, e error) {
	c := naet.NewCompiler(CompilerUrl, false)
	node := naet.NewNode(NodeUrl, false)
	ttLer := transactions.CreateTTLer(node)
	nonce := transactions.CreateNoncer(node)
	ttNonce := transactions.CreateTTLNoncer(ttLer, nonce)
	var callData = function
	if v, ok := cacheCallMap["CALL#"+function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args)]; ok {
		callData = v
	} else {
		var source []byte
		if ctID == ABCLockContractV3 {
			source, _ = ioutil.ReadFile("contract/ABCLockContractV3.aes")
		} else {
			source, _ = ioutil.ReadFile("contract/AEX9Contract.aes")
		}
		callData, _ = c.EncodeCalldata(string(source), function, args, config.CompilerBackendFATE)
		cacheCallMap["CALL#"+function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args)] = callData
	}
	data, _ := c.DecodeData(callData, "")
	println(data)

	callTx, err := transactions.NewContractCallTx(address, ctID, utils.GetRealAebalanceBigInt(amount), config.Client.Contracts.GasLimit, config.Client.GasPrice, config.Client.Contracts.ABIVersion, callData, ttNonce)
	if err != nil {
		return nil, err
	}
	return callTx, err
}

//存放调用的缓存
var cacheCallMap = make(map[string]string)

//存放解析结果的缓存
var cacheResultMap = make(map[string]interface{})

//获取合约数据try-run
func CallStaticContractFunction(address string, ctID string, function string, args []string) (s interface{}, functionEncode string, e error) {
	node := naet.NewNode(NodeUrl, false)
	compile := naet.NewCompiler(CompilerUrl, false)
	var source []byte
	if ctID == ABCLockContractV3 {
		source, _ = ioutil.ReadFile("contract/ABCLockContractV3.aes")
	} else {
		source, _ = ioutil.ReadFile("contract/AEX9Contract.aes")
	}

	var callData = ""
	if v, ok := cacheCallMap[utils.Md5V(function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args))]; ok {
		if ok && len(v) > 5 {
			callData = v

		} else {
			data, err := compile.EncodeCalldata(string(source), function, args, config.CompilerBackendFATE)
			if err != nil {
				return nil, function, err
			}
			callData = data
			cacheCallMap[utils.Md5V(function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args))] = callData
		}

	} else {
		data, err := compile.EncodeCalldata(string(source), function, args, config.CompilerBackendFATE)
		if err != nil {
			return nil, function, err
		}
		callData = data

		cacheCallMap[utils.Md5V(function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args))] = callData
	}

	callTx, err := transactions.NewContractCallTx(address, ctID, big.NewInt(0), config.Client.Contracts.GasLimit, config.Client.GasPrice, config.Client.Contracts.ABIVersion, callData, transactions.NewTTLNoncer(node))
	if err != nil {
		return nil, function, err
	}

	w := &bytes.Buffer{}
	err = callTx.EncodeRLP(w)
	if err != nil {
		println(callTx.CallData)
		return nil, function, err
	}

	txStr := binary.Encode(binary.PrefixTransaction, w.Bytes())

	body := "{\"accounts\":[{\"pub_key\":\"" + address + "\",\"amount\":100000000000000000000000000000000000}],\"txs\":[{\"tx\":\"" + txStr + "\"}]}"

	response := utils.PostBody(NodeUrlDebug+"/v2/debug/transactions/dry-run", body, "application/json")
	var tryRun TryRun
	err = json.Unmarshal([]byte(response), &tryRun)
	if err != nil {
		return nil, function, err
	}

	if v, ok := cacheResultMap[utils.Md5V(function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args))+"#"+tryRun.Results[0].CallObj.ReturnValue]; ok {
		return v, function, err
	} else {
		decodeResult, err := compile.DecodeCallResult(tryRun.Results[0].CallObj.ReturnType, tryRun.Results[0].CallObj.ReturnValue, function, string(source), config.Compiler.Backend)
		cacheResultMap[utils.Md5V(function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args))+"#"+tryRun.Results[0].CallObj.ReturnValue] = decodeResult
		return decodeResult, function, err
	}

}

var tokenCache, _ = cache.NewCache("file", `{"CachePath":"./cache","FileSuffix":".cache","DirectoryLevel":"2","EmbedExpiry":"12000"}`)

//获取代币余额调用
func TokenBalanceFunction(address string, ctID string, t string, function string, args []string) (s interface{}, functionEncode string, e error) {
	node := naet.NewNode(NodeUrl, false)
	compile := naet.NewCompiler(CompilerUrl, false)
	var source []byte
	if t == "full" {
		source, _ = ioutil.ReadFile("contract/AEX9Contract.aes")
	} else if t == "basic" {
		source, _ = ioutil.ReadFile("contract/AEX9BasicContract.aes")
	}

	var callData = ""

	if tokenCache.IsExist(utils.Md5V(function + "#" + address + "#" + ctID + "#" + fmt.Sprintf("%s", args))) {
		callData = tokenCache.Get(utils.Md5V(function + "#" + address + "#" + ctID + "#" + fmt.Sprintf("%s", args))).(string)
	} else {
		data, err := compile.EncodeCalldata(string(source), function, args, config.CompilerBackendFATE)
		if err != nil {
			return nil, function, err
		}
		callData = data
		_ = tokenCache.Put(utils.Md5V(function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args)), callData, 1000*time.Hour)
	}

	callTx, err := transactions.NewContractCallTx(address, ctID, big.NewInt(0), config.Client.Contracts.GasLimit, config.Client.GasPrice, config.Client.Contracts.ABIVersion, callData, transactions.NewTTLNoncer(node))
	if err != nil {
		return nil, function, err
	}

	w := &bytes.Buffer{}
	err = callTx.EncodeRLP(w)
	if err != nil {
		println(callTx.CallData)
		return nil, function, err
	}

	txStr := binary.Encode(binary.PrefixTransaction, w.Bytes())

	body := "{\"accounts\":[{\"pub_key\":\"" + address + "\",\"amount\":100000000000000000000000000000000000}],\"txs\":[{\"tx\":\"" + txStr + "\"}]}"

	response := utils.PostBody(NodeUrlDebug+"/v2/debug/transactions/dry-run", body, "application/json")
	var tryRun TryRun
	err = json.Unmarshal([]byte(response), &tryRun)
	if err != nil {
		return nil, function, err
	}

	if v, ok := cacheResultMap[utils.Md5V(function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args))+"#"+tryRun.Results[0].CallObj.ReturnValue]; ok {
		return v, function, err
	} else {
		decodeResult, err := compile.DecodeCallResult(tryRun.Results[0].CallObj.ReturnType, tryRun.Results[0].CallObj.ReturnValue, function, string(source), config.Compiler.Backend)
		cacheResultMap[utils.Md5V(function+"#"+address+"#"+ctID+"#"+fmt.Sprintf("%s", args))+"#"+tryRun.Results[0].CallObj.ReturnValue] = decodeResult
		return decodeResult, function, err
	}

}

type TryRun struct {
	Results []Results `json:"results"`
}
type CallObj struct {
	CallerID    string        `json:"caller_id"`
	CallerNonce int           `json:"caller_nonce"`
	ContractID  string        `json:"contract_id"`
	GasPrice    int           `json:"gas_price"`
	GasUsed     int           `json:"gas_used"`
	Height      int           `json:"height"`
	Log         []interface{} `json:"log"`
	ReturnType  string        `json:"return_type"`
	ReturnValue string        `json:"return_value"`
}
type Results struct {
	CallObj CallObj `json:"call_obj"`
	Result  string  `json:"result"`
	Type    string  `json:"type"`
}

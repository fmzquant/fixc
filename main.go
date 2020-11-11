/*
FTX exchange FIX plugin
Version : FIX 4.2
Need : fixc 
*/

package main

import (
    "hash"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "log"
    "flag"
    "encoding/json" 
    "io/ioutil"
    "time"
    "net/http"
    "strconv"
    "strings"
    "./fixc"
)

const SOH = string(1)
var _pFixClient *fixc.FIXClient
var _accessKey string 
var _secretKey string 

type RpcRequest struct {
    AccessKey string            `json:"access_key"`
    SecretKey string            `json:"secret_key"`
    Nonce     int64             `json:"nonce"`
    Method    string            `json:"method"`
    Params    map[string]string `json:"params"`
}

func HMACEncrypt(pfn func() hash.Hash, data, key string) string {
    h := hmac.New(pfn, []byte(key))
    if _, err := h.Write([]byte(data)); err == nil {
        return hex.EncodeToString(h.Sum(nil))
    }
    return ""
}

func toString(s interface{}) string {
    var ret string
    switch v := s.(type) {
    case string:
        ret = v
    case int64:
        ret = strconv.FormatInt(v, 10)
    case float64:
        ret = strconv.FormatFloat(v, 'f', -1, 64)
    case bool:
        ret = strconv.FormatBool(v)
    default:
        ret = fmt.Sprintf("%v", s)
    }
    return ret
}

func TimeStamp() string {
    t := time.Now().UTC()
    return fmt.Sprintf("%d%02d%02d-%02d:%02d:%02d", t.Year(), int(t.Month()), t.Day(), t.Hour(),t.Minute(), t.Second())   
}

// callback func
func onConnect() {
    // fix logon
    strTime := TimeStamp()
    arr := []string{strTime, "A", "1", _accessKey, "FTX"}
    data := ""
    for _, ele := range arr {
        if ele != "FTX" {
            data += ele + "\x01"
        } else {
            data += ele
        }
    }
    signature := HMACEncrypt(sha256.New, data, _secretKey)   
    // send logon msg
    if err := _pFixClient.Send(fmt.Sprintf("8=|35=A|49=|56=|34=|52=%s|98=0|108=30|96=%s|", strTime, signature)); err != nil {
        fmt.Println("err:", err)
    }
}

func onMessage(fm *fixc.FixMessage) {
    fmt.Println("receive:", fm.String())
}

func onError(err error) {
    fmt.Println(fmt.Sprintf("onError: %v", err))
    return 
}

func OnPost(w http.ResponseWriter, r *http.Request) {
    var ret interface{}
    defer func() {
        if e := recover(); e != nil {
            if ee, ok := e.(error); ok {
                e = ee.Error()
            }
            ret = map[string]string{"error": fmt.Sprintf("%v", e)}
        }

        b, _ := json.Marshal(ret)
        w.Write(b)
    }()

    b, err := ioutil.ReadAll(r.Body)
    if err != nil {
        panic(err)
    }
    var request RpcRequest
    err = json.Unmarshal(b, &request)
    if err != nil {
        panic(err)
    }

    if len(request.AccessKey) > 0 {
        _accessKey = request.AccessKey
    }
    if len(request.SecretKey) > 0 {
        _secretKey = request.SecretKey
    }
    
    var symbol string 
    if _, ok := request.Params["symbol"]; ok {
        symbol = request.Params["symbol"]
    }

    // first create FixClient
    if _pFixClient == nil {
        _pFixClient = fixc.NewFixClient(time.Second * 30, time.Second * 30, "4.2", "fix.ftx.com:4363", _accessKey, "FTX")
        // Start FixClient 
        _pFixClient.Start(onConnect, onMessage, onError)
        time.Sleep(time.Second * 2)
    }

    // processing for FMZ api , exchange.Buy / exchange.Sell , exchange.CancelOrder and so on 
    var data interface{}
    switch request.Method {
    case "trade":
        /* e.g. FTX application interface: new order 
            8=FIX.4.2|9=150|35=D|49=XXXX|56=FTX|34=2|21=1|52=20201111-03:17:14.349|
            11=fmzOrder1112|55=BTC-PERP|40=2|38=0.01|44=8000|54=1|59=1|10=078|
        */ 
        var fm *fixc.FixMessage
        if symbol == "" {
            panic("symbol is empty!")
        }
        msgType := "D"   
        tradeSids := request.Params["type"]
        if tradeSids == "buy" {
            tradeSids = "1"
        } else {
            tradeSids = "2"
        }
        ts := time.Now().UnixNano() / 1e6                                          
        msg := new(fixc.MsgBase)
        msg.AddField(35, msgType)
        msg.AddField(21, "1")
        msg.AddField(11, fmt.Sprintf("fmz%d", ts))
        msg.AddField(55, strings.ToUpper(symbol))
        msg.AddField(40, "2")
        msg.AddField(38, toString(request.Params["amount"]))
        msg.AddField(44, toString(request.Params["price"]))
        msg.AddField(54, tradeSids)
        msg.AddField(59, "1")
        _pFixClient.Send(fmt.Sprintf("8=|49=|56=|34=|52=|%s", msg.Pack()))   // new order
        fm, err = _pFixClient.Expect("35=8", "150=A")                        // waiting msg
        if err != nil {
            panic(fmt.Sprintf("%v", err))
        }
        // analysis
        if orderId, ok := fm.Find("37"); ok {
            data = map[string]string{"id": orderId}
        } else {
            panic(fmt.Sprintf("%s", fm.String()))
        }
    case "cancel":        
        orderId := request.Params["id"]
        msg := new(fixc.MsgBase)
        msg.AddField(35, "F")
        msg.AddField(37, orderId)
        _pFixClient.Send(fmt.Sprintf("8=|49=|56=|34=|52=|%s", msg.Pack()))   // cancel order 
        _, err = _pFixClient.Expect("35=8", "150=6")
        if err != nil {
            panic(fmt.Sprintf("%v", err))
        }
        data = true
    default:
        panic("FTX FIX protocol not support!")
    }

    // response to the robot request 
    ret = map[string]interface{}{
        "data": data,
    }
}

func main() {
    var addr = flag.String("b", "127.0.0.1:8888", "bind addr")
    flag.Parse()
    if *addr == "" {
        flag.Usage()
        return 
    }
    basePath := "/FTX"
    log.Println("Running ", fmt.Sprintf("http://%s%s", *addr, basePath), "...")
    http.HandleFunc(basePath, OnPost)
    http.ListenAndServe(*addr, nil)
}
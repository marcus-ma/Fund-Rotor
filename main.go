package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)



func Get(url string) string {

	// 超时时间：5秒
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	var buffer [512]byte
	result := bytes.NewBuffer(nil)
	for {
		n, err := resp.Body.Read(buffer[0:])
		result.Write(buffer[0:n])
		if err != nil && err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}
	}

	return result.String()
}

type FundInfo struct {
	//代码
	FundCode string `json:"fundcode"`
	//名字
	FundName string `json:"name"`
	//范围
	FundRange string `json:"gszzl"`
	//现在价格
	FundNow string `json:"gsz"`
	//昨天价格
	FundBefore string `json:"dwjz"`
	//范围
	Range string `json:"range"`
	//预计收益
	Money float64 `json:"money"`


}


type Connection struct {
	wsConn    *websocket.Conn
	inChan    chan []byte
	outChan   chan []byte
	closeChan chan byte

	mutex    sync.Mutex
	isClosed bool
}

func InitConnection(wsConn *websocket.Conn) (conn *Connection, err error) {
	conn = &Connection{
		wsConn:    wsConn,
		inChan:    make(chan []byte, 1000),
		outChan:   make(chan []byte, 1000),
		closeChan: make(chan byte, 1),
	}

	go conn.readLoop()
	go conn.writeLoop()

	return
}

func (conn *Connection) ReadMessage() (data []byte, err error) {
	select {
	case data = <-conn.inChan:
	case <-conn.closeChan:
		err = errors.New("connection is closed")

	}

	return
}

func (conn *Connection) WriteMessage(data []byte) (err error) {
	select {
	case conn.outChan <- data:
	case <-conn.closeChan:
		err = errors.New("connection is closed")
	}

	return
}

func (conn *Connection) Close() {
	conn.wsConn.Close()

	conn.mutex.Lock()
	if !conn.isClosed {
		close(conn.closeChan)
		conn.isClosed = true
	}
	conn.mutex.Unlock()

}

func (conn *Connection) readLoop() {
	var (
		data []byte
		err  error
	)
	for {
		if _, data, err = conn.wsConn.ReadMessage(); err != nil {
			goto ERR
		}
		select {
		case conn.inChan <- data:
		case <-conn.closeChan:
			//when closeChan closed
			goto ERR
		}

	}

ERR:
	conn.Close()
}

func (conn *Connection) writeLoop() {
	var (
		data []byte
		err  error
	)
	for {
		select {
		case data = <-conn.outChan:
		case <-conn.closeChan:
			goto ERR

		}

		if err = conn.wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
			goto ERR
		}

	}
ERR:
	conn.Close()

}

var (
	upgrade = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func wsHandler(w http.ResponseWriter, r *http.Request) {
	var (
		wsConn *websocket.Conn
		err    error
		data   []byte
		conn   *Connection
	)

	if wsConn, err = upgrade.Upgrade(w, r, nil); err != nil {
		return
	}
	if conn, err = InitConnection(wsConn); err != nil {
		goto ERR
	}


	for {
		if data, err = conn.ReadMessage(); err != nil {
			goto ERR
		}

		go func() {
			var (
				err error
			)
			for {
				if err = conn.WriteMessage(fund(string(data))); err != nil {
					return
				}
				time.Sleep(10 * time.Second)
			}

		}()
	}

ERR:
	conn.Close()
}


func fund(rcData string) []byte{

		var returnData []*FundInfo
		var fundCount int
		channel:= make(chan *FundInfo)

		jsonString := rcData
		codeMap := make(map[string][]map[string]string)
		_:json.Unmarshal([]byte(jsonString),&codeMap)

		items := codeMap["data"]
		fundCount = len(items)
		for  _,item := range items{

			go func(channel chan *FundInfo,item map[string]string) {
				res:=Get("http://fundgz.1234567.com.cn/js/"+item["code"]+".js")
				fund := &FundInfo{}
				json.Unmarshal([]byte(res[8:len(res)-2]),fund)

				FundNow,_ := strconv.ParseFloat(fund.FundNow,64)
				FundBefore,_ := strconv.ParseFloat(fund.FundBefore,64)
				HadMoney,_:=strconv.ParseFloat(item["money"],64)
				fund.Money = math.Round(((FundNow-FundBefore)*HadMoney)*100)/100
				fund.Range = fund.FundRange


				channel<-fund

			}(channel,item)


		}

		for value := range channel{
			fmt.Println(value)
			returnData = append(returnData,value)
			if len(returnData) == fundCount {close(channel)}
		}



		fmt.Println("-------------------------------------------------------------------")
		data,_ :=json.Marshal(returnData)


		return data
}





func main() {

	http.HandleFunc("/ws", wsHandler)
	fmt.Println("------------------------服务开启：端口在8888-------------------------------------------")
	http.ListenAndServe("0.0.0.0:8888", nil)

}



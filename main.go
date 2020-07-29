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

var (
	transport *http.Transport
	upgrade = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func Get(url string) string {
	// 超时时间：2秒, 连接池
	client := &http.Client{Timeout: 2 * time.Second,Transport: transport}
	resp, err := client.Get(url)
	if err != nil {
		time.Sleep(500 * time.Millisecond)
		return Get(url)
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


func wsHandler(w http.ResponseWriter, r *http.Request) {
	var (
		wsConn *websocket.Conn
		err    error
		data   []byte
		conn   *Connection
	)

	t := time.Now()

	//早上开市
	lunchStart := time.Date(t.Year(), t.Month(), t.Day(), 9, 30, 0, 0, t.Location()).Unix()
	//中午休市
	lunchClose := time.Date(t.Year(), t.Month(), t.Day(), 11, 30, 0, 0, t.Location()).Unix()
	//下午开市
	lunchOpen := time.Date(t.Year(), t.Month(), t.Day(), 13, 00, 0, 0, t.Location()).Unix()
	//下午收市
	lunchEnd := time.Date(t.Year(), t.Month(), t.Day(), 15, 00, 0, 0, t.Location()).Unix()



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

				now:=time.Now().Unix()
				if  (lunchStart < now && now < lunchClose) || (lunchOpen < now && now < lunchEnd) {
					if err = conn.WriteMessage(fund(string(data))); err != nil {
						return
					}
				}

				time.Sleep(10 * time.Second)


			}

		}()
	}

ERR:
	conn.Close()
}

//基金爬取
func fund(rcData string) []byte{

		var (
		   returnData []*FundInfo
		   fundCount int
		   sumMoney float64
		)

		jsonString := rcData
		codeMap := make(map[string][]map[string]string)
		_:json.Unmarshal([]byte(jsonString),&codeMap)

		items := codeMap["data"]
		fundCount = len(items)
		channel:= make(chan *FundInfo,fundCount)

		//记下爬取开始时间
		start := time.Now()
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
			sumMoney+=value.Money
			returnData = append(returnData,value)
			if len(returnData) == fundCount {close(channel)}
		}

		//显示预估收益
		fmt.Println("sum money:",math.Round(sumMoney*100)/100)
		//显示耗费时间
		fmt.Println("time spent:",time.Since(start).Seconds())
		fmt.Println("-------------------------------------------------------------------")
		data,_ :=json.Marshal(returnData)
		return data
}





func main() {

	transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second, //连接超时
			KeepAlive: 2 * time.Second, //探活时间
		}).DialContext,
		MaxIdleConns:          100,              //最大空闲连接
		IdleConnTimeout:       90 * time.Second, //空闲超时时间
		TLSHandshakeTimeout:   10 * time.Second, //tls握手超时时间
		ExpectContinueTimeout: 1 * time.Second,  //100-continue状态码超时时间
	}
	
	http.HandleFunc("/ws", wsHandler)
	fmt.Println("------------------------服务开启：端口在8888-------------------------------------------")
	http.ListenAndServe("0.0.0.0:8888", nil)

}



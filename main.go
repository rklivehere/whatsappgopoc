package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rhymen/go-whatsapp"
	"github.com/skip2/go-qrcode"

	"github.com/live-here/whatsapp-poc/ws"
)

var (
	textMessageList []whatsapp.TextMessage
	wac             *whatsapp.Conn
	hub             *ws.Hub
)

const (
	PORT = ":10010"
)

func main() {
	hub = ws.NewHub()
	go hub.Run()
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/read_all", readAllHandler)
	http.HandleFunc("/read", readHandler)
	http.HandleFunc("/send", sendHandler)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		setCors(w)
		ws.ServeWs(hub, w, r)
	})

	fmt.Printf("Listening on port: %s\n", PORT)
	if err := http.ListenAndServe(PORT, nil); err != nil {
		panic(err)
	}
}

func setCors(w http.ResponseWriter) {
	w.Header().Set("access-control-allow-headers", "Origin, Authorization, X-Requested-With, Content-Type, Accept, Charset")
	w.Header().Set("access-control-allow-methods", "POST, GET, OPTIONS, DELETE, PUT, PATCH")
	w.Header().Set("access-control-allow-origin", "*")
	w.Header().Set("cache-control", "private, no-cache, no-store, must-revalidate")
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if wac != nil {
		textMessageList = nil
		wac.Logout()
		fmt.Printf("Logout \n")
	}

	var err error
	wac, err = whatsapp.NewConn(10 * time.Minute)
	if err != nil {
		return
	}

	qrChan := make(chan string)

	go func() {
		_, err = wac.Login(qrChan)
		if err != nil {
			w.Write([]byte(fmt.Sprint("Internal Server Error, %s\n", err.Error())))
			w.WriteHeader(500)
			return
		}
		wac.AddHandler(whatsHandler{})
	}()

	qrCodeString := <-qrChan
	png, err := qrcode.Encode(qrCodeString, qrcode.Highest, 1024)
	if err != nil {
		w.Write([]byte(fmt.Sprint("Internal Server Error, %s\n", err.Error())))
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(png)))
	w.Write(png)
}

func readAllHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("%s %s\n", r.Method, r.URL.Path)

	if wac == nil {
		w.Write([]byte("Unauthorized"))
		w.WriteHeader(401)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&textMessageList)
}

func readHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("%s %s\n", r.Method, r.URL.Path)
	if wac == nil {
		w.Write([]byte("Unauthorized"))
		w.WriteHeader(401)
		return
	}

	phone := r.URL.Query().Get("phone")
	if strings.Compare(phone, "") == 0 {
		w.Write([]byte("Bad request"))
		w.WriteHeader(400)
		return
	}

	count := 10
	countParam := r.URL.Query().Get("count")
	if strings.Compare(countParam, "") != 0 {
		number, _ := strconv.Atoi(countParam)
		if number != 0 {
			count = number
		}
	}

	node, err := wac.LoadMessages(phone+"@s.whatsapp.net", "", count)
	if err != nil {
		w.Write([]byte(err.Error()))
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&node)
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("%s %s\n", r.Method, r.URL.Path)

	if wac == nil {
		w.Write([]byte("Unauthorized"))
		w.WriteHeader(401)
		return
	}

	text := r.URL.Query().Get("text")
	phone := r.URL.Query().Get("phone")
	if strings.Compare(text, "") == 0 || strings.Compare(phone, "") == 0 {
		w.Write([]byte("Bad request"))
		w.WriteHeader(400)
		return
	}

	body := whatsapp.TextMessage{
		Info: whatsapp.MessageInfo{
			RemoteJid: phone + "@s.whatsapp.net",
		},
		Text: text,
	}

	err := wac.Send(body)
	if err != nil {
		w.Write([]byte("Internal Server Error"))
		w.WriteHeader(500)
		return
	}

	w.Write([]byte("OK"))
	w.WriteHeader(200)
}

type whatsHandler struct{}

func (whatsHandler) HandleError(err error) {
	fmt.Fprintf(os.Stderr, "%v", err)
}

func (whatsHandler) HandleTextMessage(message whatsapp.TextMessage) {
	fmt.Println(message)
	textMessageList = append(textMessageList, message)
	reqBodyBytes := new(bytes.Buffer)
	json.NewEncoder(reqBodyBytes).Encode(message)
	for client := range hub.Clients {
		client.Send <- reqBodyBytes.Bytes()
	}
}

func (whatsHandler) HandleImageMessage(message whatsapp.ImageMessage) {
	fmt.Println(message)
}

func (whatsHandler) HandleVideoMessage(message whatsapp.VideoMessage) {
	fmt.Println(message)
}

func (whatsHandler) HandleJsonMessage(message string) {
	fmt.Println(message)
	reqBodyBytes := new(bytes.Buffer)
	json.NewEncoder(reqBodyBytes).Encode(message)
	for client := range hub.Clients {
		client.Send <- reqBodyBytes.Bytes()
	}
}

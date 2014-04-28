package wsservice

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"net/http"
)

type MsgID string

type Message struct {
	ID   MsgID
	Data *json.RawMessage
}

type Service struct {
	newConn chan<- *Connection
}

type Listener struct {
	newConn <-chan *Connection
}

func New() *Service {
	return &Service{}
}

// wsHandler handles webocket requests from the peer.
func (s *Service) wsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	ws, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(w, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		log.Println(err)
		return
	}
	c := &Connection{
		send:    make(chan *Message, 256),
		receive: make(chan *Message, 256),
		ws:      ws,
	}

	s.newConn <- c

	go c.writePump(s)
	c.readPump(s)
}

func (s *Service) Listen(addr string) *Listener {
	http.HandleFunc("/websocket/", s.wsHandler)

	newc := make(chan *Connection, 10)
	s.newConn = newc

	wsService, err := net.Listen("unix", addr)
	if err != nil {
		panic(err)
	}

	go func() { log.Fatal(http.Serve(wsService, nil)) }()

	return &Listener{newc}
}

func (l Listener) Accept() *Connection {
	return <-l.newConn
}

package wss

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type Connection struct {
	ws      *websocket.Conn
	send    chan *Message
	receive chan *Message
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Connection) readPump(s *Service) {
	defer func() {
		c.ws.Close()
		close(c.receive)
	}()
	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error { c.ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	c.ws.ReadMessage()
	var msg Message
	for {
		if err := c.ws.ReadJSON(&msg); err != nil {
			log.Println(err)
			break
		}

		switch msg.ID {
		case "token":
			token := string((*msg.Data)[1 : len(*msg.Data)-1])
			resp, err := http.PostForm("http://nerdhub.org/token",
				url.Values{"token": {token}})
			if err != nil {
				log.Println(err)
				return
			}

			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println(err)
				return
			}

			usrName := string(b)

			jret, err := json.Marshal(usrName)
			if err != nil {
				log.Println(err)
				return
			}

			retMsg := Message{ID: "login", Data: (*json.RawMessage)(&jret)}
			c.receive <- &retMsg
			c.send <- &retMsg
		default:
			c.receive <- &msg
		}
	}
}

// write writes a message with the given message type and payload.
func (c *Connection) write(mt int, payload []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(mt, payload)
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Connection) writePump(s *Service) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			buf, err := json.Marshal(message)
			if err != nil {
				return
			}
			if err := c.write(websocket.TextMessage, buf); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func (c *Connection) Send(id MsgID, data interface{}) error {
	jdata, err := json.Marshal(data)
	if err != nil {
		log.Println(err)
		return err
	}
	select {
	case c.send <- &Message{id, (*json.RawMessage)(&jdata)}:
		return nil
	default:
		return errors.New("send buffer full")
	}
}

func (c *Connection) Receive() (*Message, bool) {
	msg, ok := <-c.receive
	return msg, ok
}

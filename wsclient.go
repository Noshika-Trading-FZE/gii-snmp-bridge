package main

import (
	"time"

	"os"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var (
	// Time allowed to write a message to the peer.
	writeWait = 30 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize int64 = 8192 * 4
)

// Client is a middleman between the websocket connection and the hub
type WssClient struct {
	conn           *websocket.Conn // The websocket connection
	send           chan []byte     // Buffered channel of outbound messages
	messageHandler func(c *WssClient, received []byte)
	closed         bool
	fatal          bool // Is the closed channel fatal?
	uri            string
	// hub            *Hub
}

func NewWssClient(uri string, fatal bool, handler func(c *WssClient, received []byte)) (*WssClient, error) {

	dialer := websocket.Dialer{
		NetDial:           nil,
		Proxy:             nil,
		TLSClientConfig:   nil,
		HandshakeTimeout:  time.Second * 3,
		ReadBufferSize:    1024,
		WriteBufferSize:   1024,
		Subprotocols:      nil,
		EnableCompression: false,
		Jar:               nil,
	}

	connection, _, err := dialer.Dial(uri, nil)
	if err != nil {
		return nil, err
	}

	// connection.SetCloseHandler(func(code int, text string) error {
	// 	log.Info("WS: DISCONNECTED")
	// 	return nil
	// })

	log.Infof("WS: Connected to %s", uri)

	obj := WssClient{
		conn:           connection,
		send:           make(chan []byte, 1024),
		messageHandler: handler,
		closed:         false,
		fatal:          fatal,
		uri:            uri,
		// hub:            nil,
	}

	return &obj, nil
}

func (c *WssClient) doSend(message string) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("WS doSend (recovered): %s", r)
		}
	}()
	messageBytes := []byte(message)
	c.send <- messageBytes
}

func (c *WssClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		log.Debugf("Write pump for %s closed", c.uri)
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				log.Fatal("WS: server closed the channel")
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Errorf("Couldn't write a message, %s", err)
				return
			}

			nSent, err := w.Write(message)
			if err != nil {
				log.Errorf("Couldn't write a message, %s", err)
			} else {
				log.Debugf("WS.SENT: %d bytes, %s", nSent, message)
			}

			if err := w.Close(); err != nil {
				log.Errorf("WS.write: %s", err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if c.closed {
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				if snmp != nil {
					snmp.Stream <- snmp.TrapLNS(true)
					time.Sleep(1 * time.Second)
				}

				log.Warn("This is not a crash or panic. Bridge is trying to restat ASAP.")
				log.Errorf("WS (%s): Connection ping timeout.", c.uri)
				os.Exit(2)
				return
			}
		}
	}
}

func (c *WssClient) readPump() {
	defer func() {
		c.conn.Close()
		log.Debugf("Read pump for %s closed", c.uri)
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		log.Debug("Pong recieved")
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			// 	log.Error(err)
			// }
			c.closed = true
			log.Errorf("WS connection %s, %v", c.uri, err)
			if c.fatal {
				if snmp != nil {
					snmp.Stream <- snmp.TrapLNS(true)
					time.Sleep(1 * time.Second)
				}

				log.Warn("This is not a crash or panic. Bridge is trying to restat ASAP.")
				log.Error("Restart")
				os.Exit(2)
			}
			break
		}

		// handle received message
		c.messageHandler(c, message)
	}
}

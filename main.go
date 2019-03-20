package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-logfmt/logfmt"
	"github.com/gorilla/websocket"
	"github.com/oklog/run"
)

var (
	addr = flag.String("addr", "0.0.0.0:8080", "http service address")

	multiplexer = &Multiplexer{
		broadcast: make(chan []byte),
		clients:   make(map[*Client]bool),
	}

	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "home.html")
}

func serveWebsocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("websocket upgrade failed:", err)
		return
	}

	client := &Client{conn: conn, send: make(chan []byte, maxMessageSize)}
	multiplexer.Register(client)

	go client.pump()
}

func transcode() error {
	decoder := logfmt.NewDecoder(os.Stdin)
	encoder := kitlog.NewJSONLogger(kitlog.NewSyncWriter(multiplexer))

	for decoder.ScanRecord() {
		var kvs []interface{}
		for decoder.ScanKeyval() {
			kvs = append(kvs, string(decoder.Key()), string(decoder.Value()))
		}

		if err := encoder.Log(kvs...); err != nil {
			return err
		}
	}

	return nil
}

func interrupt(cancel chan struct{}) error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-c:
		return errors.New("interrupted")
	case <-cancel:
		return errors.New("canceled")
	}
}

func main() {
	// info, err := os.Stdin.Stat()
	// if err != nil {
	// 	panic(err)
	// }

	// if info.Mode()&os.ModeCharDevice != 0 || info.Size() <= 0 {
	// 	fmt.Println("Slurpily is intended to work with pipes.")
	// 	fmt.Println("Usage: echo \"log=Hello\" | slurpily")
	// 	return
	// }

	var g run.Group
	server := http.Server{Addr: *addr}

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", serveWebsocket)

	cancel := make(chan struct{})

	g.Add(transcode, func(error) {})
	g.Add(func() error { return multiplexer.run(cancel) }, func(error) {})
	g.Add(func() error { return interrupt(cancel) }, func(err error) { close(cancel) })
	g.Add(server.ListenAndServe, func(error) { server.Close() })

	if err := g.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
}

func (c *Client) pump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		multiplexer.Unregister(c)
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

type Multiplexer struct {
	clients   map[*Client]bool
	broadcast chan []byte
	mutex     sync.Mutex
}

func (m *Multiplexer) Register(c *Client) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.clients[c] = true
}

func (m *Multiplexer) Unregister(c *Client) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, ok := m.clients[c]; ok {
		delete(m.clients, c)
		close(c.send)
	}
}

func (m *Multiplexer) Write(p []byte) (int, error) {
	m.broadcast <- p
	return len(p), nil
}

func (m *Multiplexer) run(stop chan struct{}) error {
	for {
		select {
		case msg := <-m.broadcast:
			for c := range m.clients {
				select {
				case c.send <- msg:
				default:
					close(c.send)
					delete(m.clients, c)
				}
			}
		case <-stop:
			for c := range m.clients {
				close(c.send)
				delete(m.clients, c)
			}
			return nil
		}
	}

	return nil
}

package web

import (
	"crypto/tls"

	"github.com/gorilla/websocket"
)

func NewWsStream(url string, config *tls.Config) (*WsServiceStream, error) {
	logger.Debugf("Connecting to %s", url)
	dialer := &websocket.Dialer{TLSClientConfig: config}
	ws, _, err := dialer.Dial(url, nil)
	logger.Infof("Successfully connected to websocket")
	if err != nil {
		logger.Errorf("Dial failed: %s\n", err.Error())
		return nil, err
	}
	return &WsServiceStream{conn: ws}, nil
}

type WsServiceStream struct {
	conn *websocket.Conn
}

func (c *WsServiceStream) Send(v interface{}) error {
	logger.Debugf("Sending to websocket: %v", v)
	return c.conn.WriteJSON(v)
}
func (c *WsServiceStream) Recv() (interface{}, error) {
	var v interface{}
	err := c.conn.ReadJSON(&v)
	logger.Debugf("Received from websocket: %v", v)
	return v, err
}
func (c *WsServiceStream) Close() error {
	return c.conn.Close()
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package sender

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const maxConnectionAttempts = 5
const backoffSleepTime = 2
const timeout = 20 * time.Second

// A ConnectionManager manages connections
type ConnectionManager struct {
	connectionString    string
	serverName          string
	skip_ssl_validation bool

	mutex   sync.Mutex
	retries int

	firstConn bool
}

// NewConnectionManager returns an initialized ConnectionManager
func NewConnectionManager(ddUrl string, ddPort int, skip_ssl_validation bool) *ConnectionManager {
	return &ConnectionManager{
		connectionString:    fmt.Sprintf("%s:%d", ddUrl, ddPort),
		serverName:          ddUrl,
		skip_ssl_validation: skip_ssl_validation,

		mutex:   sync.Mutex{},
		retries: maxConnectionAttempts,

		firstConn: true,
	}
}

// NewConnection returns an initialized connection to the intake.
// It blocks until a connection is available
func (cm *ConnectionManager) NewConnection() (net.Conn, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for cm.retries > 0 {
		if cm.firstConn {
			log.Println("Connecting to the backend:", cm.connectionString, "- skip_ssl_validation:", cm.skip_ssl_validation)
			cm.firstConn = false
		}

		cm.retries -= 1
		outConn, err := net.DialTimeout("tcp", cm.connectionString, timeout)
		if err != nil {
			reportNetError("out_connection.connection_error", err)
			cm.backoff()
			continue
		}

		if !cm.skip_ssl_validation {
			config := &tls.Config{
				ServerName: cm.serverName,
			}
			sslConn := tls.Client(outConn, config)
			err = sslConn.Handshake()
			if err != nil {
				reportNetError("out_connection.handshake_error", err)
				cm.backoff()
				continue
			}
			outConn = sslConn
		}

		cm.retries = maxConnectionAttempts
		go cm.handleServerClose(outConn)
		return outConn, nil
	}

	cm.retries = 1 // we know we're in a degraded state, we don't need to retry often
	return nil, fmt.Errorf("Connection failed")
}

// CloseConnection closes a connection on the client side
func (cm *ConnectionManager) CloseConnection(conn net.Conn) {
	conn.Close()
}

// handleServerClose lets the connection manager detect when a connection
// has been closed by the server, and closes it for the client.
func (cm *ConnectionManager) handleServerClose(conn net.Conn) {
	for {
		buff := make([]byte, 1)
		_, err := conn.Read(buff)
		if err == io.EOF {
			cm.CloseConnection(conn)
			return
		} else if err != nil {
			switch err.(type) {
			case *net.OpError:
				if err.(*net.OpError).Err.Error() == "use of closed network connection" {
					log.Println("Client closed the connection")
					return
				}
			}
			panic(err) // fixme: close connection
		}
	}
}

// backoff lets the connection mananger sleep a bit
func (cm *ConnectionManager) backoff() {
	timer := time.NewTimer(time.Second * time.Duration((maxConnectionAttempts-cm.retries)*backoffSleepTime))
	<-timer.C
}

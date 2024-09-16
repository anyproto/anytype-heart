package localdiscovery

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"
)

const expectedMessage = "test"

func handleConnection(conn net.Conn) error {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	message, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading message: %w", err)
	}

	// Trim newline characters and validate the message
	message = strings.TrimSpace(message)

	if message != expectedMessage {
		return fmt.Errorf("unexpected message received: %s", message)
	}

	return nil
}

func startServer(ip string) (listener net.Listener, port int, err error) {
	listener, err = net.Listen("tcp", ip+":0")
	if err != nil {
		return nil, 0, fmt.Errorf("error starting server: %w", err)
	}

	port = listener.Addr().(*net.TCPAddr).Port

	return listener, port, nil
}

func sendMessage(ip string, port int, message string) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), 3*time.Second)
	if err != nil {
		return fmt.Errorf("error connecting: %w", err)
	}
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "%s\n", message)
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}

	return nil
}

func testSelfConnection(ip string) error {
	listener, port, err := startServer(ip)
	if err != nil {
		return err
	}
	var err2 error
	defer listener.Close()
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		for {
			conn, err := listener.Accept()
			if err != nil {
				err2 = err
				return
			}
			err2 = handleConnection(conn)
			return
		}
	}()
	err = sendMessage(ip, port, expectedMessage)
	if err != nil {
		return err
	}
	<-ch
	return err2
}

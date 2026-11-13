package mailer

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

func TestSendRejectsPlaintextSMTP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		conn.Write([]byte("220 test SMTP\r\n"))
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "EHLO ") {
			done <- err
			return
		}
		conn.Write([]byte("250 test SMTP\r\n"))
		done <- nil
	}()
	addr := listener.Addr().(*net.TCPAddr)
	mail := New("127.0.0.1", strings.TrimPrefix(addr.String(), "127.0.0.1:"), "", "", "sender@example.invalid")
	if err := mail.Send("receiver@example.invalid", "verify", "body", false); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected STARTTLS error, got %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

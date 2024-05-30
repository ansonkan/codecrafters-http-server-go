package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
)

func main() {
	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	conn, err := l.Accept()
	if err != nil {
		fmt.Println("Error accepting connection: ", err.Error())
		os.Exit(1)
	}

	buf := make([]byte, 1024)
	tmp := make([]byte, 0, 1024)

	for {
		n, err := conn.Read(buf)
		tmp = append(tmp, buf[:n]...)

		if n < len(buf) {
			break
		}

		if err != nil {
			if err != io.EOF {
				fmt.Printf("Read error: %v\n", err)
			}
			break
		}
	}

	req_parts := strings.Split(string(tmp), "\r\n")
	line_parts := strings.Split(req_parts[0], " ")

	method := line_parts[0]
	target := line_parts[1]

	r_echo, _ := regexp.Compile("^/echo(/.*)?$")

	switch method {
	case "GET":
		switch {
		case target == "/":
			conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		case r_echo.MatchString(target):
			body := ""

			matches := r_echo.FindStringSubmatch(target)
			if len(matches) == 2 {
				body = matches[1][1:]
			}

			conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(body), body)))
		default:
			conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		}
	default:
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
	}

	conn.Close()
}

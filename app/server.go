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

	request := string(tmp)

	req_line_end := strings.Index(request, "\r\n")
	req_line_parts := strings.Split(request[:req_line_end], " ")

	method := req_line_parts[0]
	target := req_line_parts[1]

	headers_parts := strings.Split(request[req_line_end+len("\r\n"):strings.Index(request, "\r\n\r\n")], "\r\n")
	r_header, _ := regexp.Compile("^([a-zA-z0-9-_]+): (.+)?$")
	headers := make(map[string]string) // all lower case
	for _, v := range headers_parts {
		matches := r_header.FindStringSubmatch(v)
		if len(matches) == 3 {
			headers[strings.ToLower(matches[1])] = matches[2]
		}
	}

	r_echo, _ := regexp.Compile("^/echo(/.*)?$")

	switch method {
	case "GET":
		switch {
		case target == "/":
			conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		case target == "/user-agent":
			body := headers["user-agent"]

			conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(body), body)))
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

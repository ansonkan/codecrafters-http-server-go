package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
)

const (
	read_file_buf_size = 1024
)

var (
	r_header    = regexp.MustCompile("^([a-zA-z0-9-_]+): (.+)?$")
	r_path_echo = regexp.MustCompile("^/echo(/.*)?$")
	r_path_file = regexp.MustCompile("^/files/(.+)$")
)

func main() {
	directory := ""
	for i, v := range os.Args {
		if v == "--directory" && i+1 <= len(os.Args) {
			directory = os.Args[i+1]
		}
	}

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}

		go handleConnection(&conn, &directory)
	}
}

func handleConnection(c *net.Conn, dir *string) {
	defer (*c).Close()

	buf := make([]byte, 1024)
	tmp := make([]byte, 0, 1024)

	for {
		n, err := (*c).Read(buf)
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

	header_suffix_index := strings.Index(request, "\r\n\r\n")

	headers_parts := strings.Split(request[req_line_end+len("\r\n"):header_suffix_index], "\r\n")
	headers := make(map[string]string) // all lower case
	for _, v := range headers_parts {
		matches := r_header.FindStringSubmatch(v)
		if len(matches) == 3 {
			headers[strings.ToLower(matches[1])] = matches[2]
		}
	}

	// TODO: would be great it request body is read by chunks and not loading all of them into memory in case of big request body
	req_body := request[header_suffix_index+len("\r\n\r\n"):]

	response := []byte("HTTP/1.1 404 Not Found\r\n\r\n")

	switch method {
	case "GET":
		switch {
		case target == "/":
			response = []byte("HTTP/1.1 200 OK\r\n\r\n")

		case target == "/user-agent":
			body := headers["user-agent"]
			response = []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(body), body))

		case r_path_echo.MatchString(target):
			body := ""

			matches := r_path_echo.FindStringSubmatch(target)
			if len(matches) == 2 {
				body = matches[1][1:]
			}

			response = []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(body), body))

		case r_path_file.MatchString(target):
			matches := r_path_file.FindStringSubmatch(target)

			if len(matches) != 2 {
				break
			}

			f, err := os.Open(path.Join(*dir, matches[1]))
			if err != nil {
				fmt.Printf("Error opening file: %v\n", err)
				break
			}
			defer f.Close()

			f_info, err := f.Stat()
			if err != nil {
				fmt.Printf("Error getting file stat: %v\n", err)
				break
			}

			(*c).Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n", f_info.Size())))

			var seek_offset int64 = 0
			var seek_err error
			var read_err error

			buf := make([]byte, read_file_buf_size)

			for {
				_, seek_err = f.Seek(seek_offset, 0)
				Check(seek_err)

				_, read_err = f.Read(buf)
				if read_err == io.EOF {
					break
				}
				Check(read_err)

				(*c).Write(buf)

				seek_offset += read_file_buf_size
			}

			// TODO: would be great if not just this GET /files/{file_name} path has a different flow
			return
		}
	case "POST":
		switch {
		case r_path_file.MatchString(target):
			matches := r_path_file.FindStringSubmatch(target)

			if len(matches) != 2 {
				break
			}

			err := os.WriteFile(path.Join(*dir, matches[1]), []byte(req_body), 0644)
			if err != nil {
				response = []byte("HTTP/1.1 500 Internal Server Error\r\n\r\n")
				break
			}

			response = []byte("HTTP/1.1 201 Created\r\n\r\n")
		}
	}

	(*c).Write(response)
}

func Check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

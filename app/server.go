package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

const (
	read_file_buf_size = 1024
)

var (
	valid_encoding_schemes = []string{"gzip"}
	res_status_description = map[int]string{
		200: "OK",
		201: "Created",
		404: "Not Found",
		500: "Internal Server Error",
	}
	r_header    = regexp.MustCompile("^([a-zA-z0-9-_]+): (.+)?$")
	r_path_echo = regexp.MustCompile("^/echo(/.*)?$")
	r_path_file = regexp.MustCompile("^/files/(.+)$")
)

type Headers = map[string]string

// TODO: better struct? When to use pointer and when not to?
type Response struct {
	c            *net.Conn
	http_version string
	status       int
	headers      Headers
	body         string
}

// TODO: better name?
func (res *Response) writeResponse() {
	// status
	(*res.c).Write([]byte(fmt.Sprintf("%s %d %s\r\n", res.http_version, res.status, res_status_description[res.status])))

	// encoding
	switch res.headers["content-encoding"] {
	case "gzip":
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)

		_, err := zw.Write([]byte(res.body))
		if err != nil {
			log.Fatal(err)
		}
		zw.Close()

		res.body = buf.String()
		res.headers["content-length"] = fmt.Sprintf("%d", buf.Len())
	}

	// headers
	for k, v := range res.headers {
		(*res.c).Write([]byte(fmt.Sprintf("%s: %s\r\n", strings.ToLower(k), v)))
	}
	(*res.c).Write([]byte("\r\n"))

	// body
	(*res.c).Write([]byte(res.body))
}

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

	// TODO: what if it is not a valid HTTP request? What to response?
	method := req_line_parts[0]
	target := req_line_parts[1]
	http_version := req_line_parts[2]

	header_suffix_index := strings.Index(request, "\r\n\r\n")

	req_headers_parts := strings.Split(request[req_line_end+len("\r\n"):header_suffix_index], "\r\n")
	req_headers := make(Headers) // all lower case
	for _, v := range req_headers_parts {
		matches := r_header.FindStringSubmatch(v)
		if len(matches) == 3 {
			req_headers[strings.ToLower(matches[1])] = matches[2]
		}
	}

	// TODO: would be great it request body is read by chunks and not loading all of them into memory in case of big request body
	req_body := request[header_suffix_index+len("\r\n\r\n"):]

	res := Response{c: c, http_version: http_version, status: 404, headers: make(Headers)}

	if accept_encoding, ok := req_headers["accept-encoding"]; ok {
		for _, v := range strings.Split(accept_encoding, ",") {
			if encoding := strings.Trim(v, " "); slices.Contains(valid_encoding_schemes, encoding) {
				res.headers["content-encoding"] = encoding
				break
			}
		}
	}

	switch method {
	case "GET":
		switch {
		case target == "/":
			res.status = 200

		case target == "/user-agent":
			res.status = 200
			res.body = req_headers["user-agent"]
			res.headers["content-type"] = "text/plain"
			res.headers["content-length"] = fmt.Sprintf("%d", len(res.body))

		case r_path_echo.MatchString(target):
			res.status = 200

			matches := r_path_echo.FindStringSubmatch(target)
			if len(matches) == 2 && matches[1] != "" {
				res.body = matches[1][1:]
			}

			res.headers["content-type"] = "text/plain"
			res.headers["content-length"] = fmt.Sprintf("%d", len(res.body))

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

			is_gzip := res.headers["content-encoding"] == "gzip"
			zw := gzip.NewWriter(*res.c)
			defer zw.Close()

			/*
				CURL "--compressed" options can decode the gzipped response body correctly:
				curl -i --header "Accept-Encoding: gzip" --compressed http://localhost:4221/files/small_text_file.txt

				Guess "content-length" is really optional.
				While "Transfer-Encoding: chunked" seems unrelated because the gzipping body here is not in chunks that
				separated by "\r\n", https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Transfer-Encoding
			*/
			res.status = 200
			res.headers["content-type"] = "application/octet-stream"
			if !is_gzip {
				res.headers["content-length"] = fmt.Sprintf("%d", f_info.Size())
			}

			// TODO: merge this gzip file streaming logic with struct "Response"
			// status
			(*res.c).Write([]byte(fmt.Sprintf("%s %d %s\r\n", res.http_version, res.status, res_status_description[res.status])))

			// headers
			for k, v := range res.headers {
				(*res.c).Write([]byte(fmt.Sprintf("%s: %s\r\n", strings.ToLower(k), v)))
			}
			(*res.c).Write([]byte("\r\n"))

			var seek_offset int64 = 0
			buf := make([]byte, read_file_buf_size)

			for {
				_, seek_err := f.Seek(seek_offset, 0)
				check(seek_err)

				n, read_err := f.Read(buf)
				if read_err == io.EOF {
					break
				}
				check(read_err)

				// how to encode this with gzip?
				if is_gzip {
					_, err := zw.Write(buf[:n])
					if err != nil {
						fmt.Println("Error writing gzipped bytes to connection:", err)
						return
					}

				} else {
					(*res.c).Write(buf)
				}

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

			// TOOD: read request body, and write file in chunks
			err := os.WriteFile(path.Join(*dir, matches[1]), []byte(req_body), 0644)
			if err != nil {
				res.status = 500
				break
			}

			res.status = 201
		}
	}

	res.writeResponse()
}

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

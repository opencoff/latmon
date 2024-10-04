package main

import (
	"os"
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
	"time"

	"context"
	"math/rand/v2"
	"net/url"
	"strconv"
)

// HTTPRequest structure to hold HTTP method, headers, and body
type HTTPRequest struct {
	Method  string
	URL     string
	Host    string
	Headers map[string]string
	Body    string
}

// HTTPClient to handle connections and requests
type HTTPClient struct {
	Timeout time.Duration
}

// NewHTTPClient creates a new HTTP client with a specified timeout
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		Timeout: timeout,
	}
}

// Do sends an HTTP request and returns the response as a string
func (c *HTTPClient) Do(req *HTTPRequest, ctx context.Context) (*Response, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("http: url %s: %w", req.URL, err)
	}

	var port int
	host := u.Hostname()
	if u.Port() == "" {
		switch u.Scheme {
		case "https":
			port = 443
		case "http":
			port = 80
		default:
			panic(fmt.Sprintf("don't know how to handle scheme %s", u.Scheme))
		}
	} else {
		px, err := strconv.ParseUint(u.Port(), 0, 16)
		if err != nil {
			return nil, fmt.Errorf("http: url %s: %w", req.URL, err)
		}
		port = int(px)
	}

	req.Host = host

	// see if "host" is an IP address or name
	ip := net.ParseIP(host)
	if ip == nil {
		ip, err = c.resolve(host, ctx)
		if err != nil {
			return nil, fmt.Errorf("http: dns: %s: %w", host, err)
		}
	}

	taddr := &net.TCPAddr{
		IP:   ip,
		Port: port,
	}

	fmt.Printf("Connecting to %s .. \n", taddr)

	var conn net.Conn
	conn, err = net.DialTCP("tcp", nil, taddr)
	if err != nil {
		return nil, fmt.Errorf("http: dial %s: %w", taddr, err)
	}

	if u.Scheme == "https" {
		// now setup TLS
		tcfg := &tls.Config{
			ServerName: host,
		}

		tconn := tls.Client(conn, tcfg)
		if err = tconn.Handshake(); err != nil {
			return nil, fmt.Errorf("http: tls %s: %w", taddr, err)
		}

		fmt.Printf("+tls ok\n")
		conn = tconn
	}

	// Build the HTTP request manually
	httpRequest := buildHTTPRequest(req, u.RequestURI())

	// Write the request to the connection
	_, err = conn.Write([]byte(httpRequest))
	if err != nil {
		return nil, fmt.Errorf("http: write %s: %w", host, err)
	}

	fmt.Printf("+waiting for response ..\n")

	resp := &Response{
		Req: req,
	}

	// Read the response from the connection
	if err = readResponse(conn, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *HTTPClient) resolve(host string, ctx context.Context) (net.IP, error) {
	r := &net.Resolver{
		PreferGo: true,
	}

	ips, err := r.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil, fmt.Errorf("http: %s: %w", host, err)
	}

	// pick a random IP addr
	i := rand.IntN(len(ips))
	return ips[i], nil
}

// buildHTTPRequest creates the raw HTTP request string
func buildHTTPRequest(req *HTTPRequest, path string) string {
	// Start with the request line
	request := fmt.Sprintf("%s %s HTTP/1.1\r\n", req.Method, path)

	// Add required headers
	request += fmt.Sprintf("Host: %s\r\n", req.Host)
	for key, value := range req.Headers {
		request += fmt.Sprintf("%s: %s\r\n", key, value)
	}

	// Add content length if the body is provided
	if req.Body != "" {
		request += fmt.Sprintf("Content-Length: %d\r\n", len(req.Body))
	}

	// End the headers
	request += "\r\n"

	// Add the body if it's provided
	if req.Body != "" {
		request += req.Body
	}

	return request
}

func readResponse(conn net.Conn, r *Response) error {
	rd := bufio.NewReader(conn)
	tr := textproto.NewReader(rd)

	// parse the first line
	line, err := tr.ReadLine()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return err
	}

	proto, status, ok := strings.Cut(line, " ")
	if !ok {
		return fmt.Errorf("http: malformed response line: %s", line)
	}

	r.Proto = proto
	r.Status = strings.TrimLeft(status, "")

	statusCode, _, _ := strings.Cut(r.Status, " ")
	if len(statusCode) != 3 {
		return fmt.Errorf("malformed HTTP status code: %s", r.Status)
	}

	r.StatusCode, err = strconv.Atoi(statusCode)
	if err != nil || r.StatusCode < 0 {
		return fmt.Errorf("malformed status code: %s", r.Status)
	}

	mh, err := tr.ReadMIMEHeader()
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return err
	}

	r.Header = Header(mh)

	fmt.Printf("%s %s -- got headers\n", r.Proto, r.Status)
	for k, v := range mh {
		fmt.Printf("%s: %s\n", k, strings.Join(v, ";"))
	}

	var body io.ReadCloser

	if enc, ok := mh["Transfer-Encoding"]; ok && has(enc, "chunked") {
		fmt.Printf("using chunked encoding ..\n")
		body = NewChunkedReader(rd, conn)
		//body = NewSimpleReader(rd, conn)
	} else {
		fmt.Printf("content-length: %v\n", mh["Content-Length"])
		body = NewSimpleReader(rd, conn)
	}
	r.Body = body
	return nil
}

type Header map[string][]string

func has(stack []string, needle string) bool {
	for i := range stack {
		if stack[i] == needle {
			return true
		}
	}
	return false
}

type Response struct {
	Req *HTTPRequest

	Proto      string
	Status     string
	StatusCode int

	Header Header

	Body io.ReadCloser
}

type simpleReader struct {
	rd *bufio.Reader
	raw io.ReadCloser
}

func NewSimpleReader(rd *bufio.Reader, raw io.ReadCloser) *simpleReader {
	return &simpleReader{
		rd: rd,
		raw: raw,
	}
}

func (c *simpleReader) Close() error {
	fmt.Printf("simple reader closed\n")
	return c.raw.Close()
}

func (c *simpleReader) Read(p []byte) (int, error) {
	return c.rd.Read(p)
}

// ChunkedReader struct to handle chunked transfer encoding
type ChunkedReader struct {
	reader *bufio.Reader
	raw io.ReadCloser
}

// NewChunkedReader creates a new ChunkedReader from a connection or any io.Reader
func NewChunkedReader(rd *bufio.Reader, raw io.ReadCloser) *ChunkedReader {
	return &ChunkedReader{
		reader: rd,
		raw: raw,
	}
}

func (c *ChunkedReader) Close() error {
	return c.raw.Close()
}

// Read implements the io.Reader interface for ChunkedReader
// It reads and decodes the chunked transfer encoding from the stream
func (cr *ChunkedReader) Read(p []byte) (int, error) {
	var tot int

	var i int
	want := len(p)
	b := p

	for want > 0 {
		n, err := cr.readChunkSize()
		if err != nil {
			return tot, err
		}
		if n == 0 {
			return tot, io.EOF
		}

		if n > want {
			// this is an error.
			// We don't have a buffer large enough
			// but we've already read the chunk size!
			return tot, fmt.Errorf("chunked-reader: buffer too small (%d bytes of %d left); want %d",
							want, len(p), n)
		}

		fmt.Printf("Chunk #%d: %d bytes\n", i, n)

		// Read the chunk data
		_, err = io.ReadFull(cr.reader, b[:n])
		if err != nil {
			return tot, fmt.Errorf("chunked-reader: chunk data: %w", err)
		}

		fmt.Printf("|")
		os.Stdout.Write(b)
		fmt.Printf("|\n")

		// now skip the crlf
		if err = cr.skipCRLF(); err != nil {
			return tot, fmt.Errorf("chunked-reader: crlf: %w", err)
		}
		want -= n
		b = b[n:]
		i += 1
	}
	return len(p), nil
}

// readChunkSize reads the chunk size from the stream (hexadecimal format)
func (cr *ChunkedReader) readChunkSize() (int, error) {
	line, err := cr.reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	size, err := strconv.ParseInt(line, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chunk size: %v", err)
	}
	return int(size), nil
}

// skipCRLF skips the CRLF after a chunk
func (cr *ChunkedReader) skipCRLF() error {
	var buf [2]byte

	n, err := io.ReadFull(cr.reader, buf[:])
	if err != nil {
		return err
	}


	fmt.Printf("CRLF: %d bytes => |%x|\n", n, buf[:n])

	crlf := string(buf[:n])
	if crlf != "\r\n" {
		return errors.New("invalid CRLF after chunk")
	}
	return nil
}

// Example usage for GET, POST, PUT, DELETE, PATCH
func main() {
	client := NewHTTPClient(10 * time.Second)

	// Example: GET request
	getRequest := &HTTPRequest{
		Method: "GET",
		URL:    "https://www.google.com",
		Headers: map[string]string{
			"User-Agent": "Custom-Client",
			"Connection": "close",
		},
	}

	ctx := context.Background()
	response, err := client.Do(getRequest, ctx)
	if err != nil {
		if err != io.EOF {
			fmt.Printf("Error: %v\n", err)
		}
	} else {
		fmt.Printf("Success! Body:\n")


		buf := make([]byte, 1048576)
		rd := response.Body
		for {
			n, err := rd.Read(buf[:])
			if err != nil {
				if err != io.EOF {
					fmt.Printf("\n\nError reading body: %s\n", err)
					os.Exit(1)
				}
			}
			if n == 0 {
				break
			}
			os.Stdout.Write(buf[:n])
		}

	}
	response.Body.Close()

	/*
		// Example: POST request
		postRequest := &HTTPRequest{
			Method: "POST",
			URL:    "http://example.com",
			Headers: map[string]string{
				"User-Agent":   "Custom-Client",
				"Content-Type": "application/x-www-form-urlencoded",
			},
			Body: "key=value",
		}
		response, err = client.Do(postRequest)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("POST Response:\n%s\n", response)
		}

		// Similar logic can be applied for PUT, DELETE, PATCH requests.
	*/
}

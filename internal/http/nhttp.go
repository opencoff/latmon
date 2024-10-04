package http

import (
	"bufio"
	"crypto/tls"
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
	Headers Header
	Body    string
}

type Header map[string][]string

type Response struct {
	Req *HTTPRequest

	Proto      string
	Status     string
	StatusCode int

	Headers Header

	Body io.ReadCloser

	// raw underlying connections
	tls  *tls.Conn
	conn net.Conn
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
	var tconn *tls.Conn
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

		tls:  tconn,
		conn: conn,
	}

	rx := newConnCloser(conn)
	// Read the response from the connection
	if err = readResponse(rx, resp); err != nil {
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

func readResponse(rd *connCloser, r *Response) error {
	tr := textproto.NewReader(rd.Reader)

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

	r.Headers = Header(mh)

	fmt.Printf("%s %s -- got headers\n", r.Proto, r.Status)
	for k, v := range mh {
		fmt.Printf("%s: %s\n", k, strings.Join(v, ";"))
	}

	if enc, ok := mh["Transfer-Encoding"]; ok && has(enc, "chunked") {
		fmt.Printf("using chunked encoding ..\n")
		r.Body = NewChunkedStreamReader(rd)
		//body = NewSimpleReader(rd, conn)
	} else {
		fmt.Printf("content-length: %v\n", mh["Content-Length"])
		r.Body = rd
	}
	return nil
}

func has(stack []string, needle string) bool {
	for i := range stack {
		if stack[i] == needle {
			return true
		}
	}
	return false
}

type connCloser struct {
	*bufio.Reader
	conn net.Conn
}

func newConnCloser(conn net.Conn) *connCloser {
	r := &connCloser{
		Reader: bufio.NewReader(conn),
		conn:   conn,
	}
	return r
}

func (c *connCloser) Read(p []byte) (int, error) {
	return c.Reader.Read(p)
}

func (c *connCloser) Close() error {
	return c.conn.Close()
}

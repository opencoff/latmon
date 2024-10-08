// http.go - simple http client to aid in timing measurements
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

	nh "net/http"
)

type Header = nh.Header

// Request structure to hold HTTP method, headers, and body
type Request struct {
	Method  string
	URL     string
	Host    string
	Headers Header
}

func NewRequest(meth, url string) *Request {
	r := &Request{
		Method:  meth,
		URL:     url,
		Headers: make(Header),
	}
	return r
}

func (r *Request) write(conn net.Conn, uri string) error {
	// make a big enough buffer
	b := bufio.NewWriterSize(conn, 4096)

	// Start with the request line
	_, err := fmt.Fprintf(b, "%s %s HTTP/1.1\r\n", r.Method, uri)
	if err != nil {
		return err
	}

	// Add required headers
	if vals := r.Headers.Get("host"); len(vals) == 0 {
		r.Headers.Set("host", r.Host)
	}

	if err = r.Headers.Write(b); err != nil {
		return err
	}

	if _, err = fmt.Fprintf(b, "\r\n"); err != nil {
		return err
	}

	return b.Flush()
}

type Response struct {
	Req *Request

	Proto      string
	Status     string
	StatusCode int

	Headers Header

	Body io.ReadCloser

	// various timings
	Dns  time.Duration
	Tcp  time.Duration
	Tls  time.Duration
	Http time.Duration
	E2e  time.Duration

	// raw underlying connections
	tls  *tls.Conn
	conn net.Conn
}

// Client to handle connections and requests
type Client struct {
	Timeout time.Duration

	resolv net.Resolver
}

// NewClient creates a new HTTP client with a specified timeout
func NewClient(timeout time.Duration) *Client {
	return &Client{
		Timeout: timeout,
		resolv: net.Resolver{
			PreferGo: true,
		},
	}
}

// Do sends an HTTP request and returns the response as a string
func (c *Client) Do(req *Request, ctx context.Context) (*Response, error) {
	start := time.Now()

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

	var dns, tcp, ttls, http time.Duration

	// see if "host" is an IP address or name
	ip := net.ParseIP(host)
	if ip == nil {
		st := time.Now()
		ip, err = c.resolve(host, ctx)
		if err != nil {
			return nil, fmt.Errorf("http: dns: %s: %w", host, err)
		}
		dns = time.Now().Sub(st)
	}

	taddr := &net.TCPAddr{
		IP:   ip,
		Port: port,
	}

	var conn net.Conn
	var tconn *tls.Conn
	st := time.Now()
	conn, err = net.DialTCP("tcp", nil, taddr)
	if err != nil {
		return nil, fmt.Errorf("http: dial %s (%s): %w", host, taddr, err)
	}
	tcp = time.Now().Sub(st)

	if u.Scheme == "https" {
		st := time.Now()
		// now setup TLS
		tcfg := &tls.Config{
			ServerName: host,
		}

		tconn := tls.Client(conn, tcfg)

		c.setDeadline(tconn)
		if err = tconn.Handshake(); err != nil {
			return nil, fmt.Errorf("http: tls %s: %w", taddr, err)
		}
		conn = tconn
		ttls = time.Now().Sub(st)
	}

	// Build the HTTP request manually
	c.setWriteDeadline(conn)
	st = time.Now()
	err = req.write(conn, u.RequestURI())
	if err != nil {
		return nil, fmt.Errorf("http: write %s: %w", host, err)
	}

	resp := &Response{
		Req:  req,
		tls:  tconn,
		conn: conn,
	}

	// Read the response from the connection
	c.setReadDeadline(conn)
	rx := newConnCloser(conn)
	if err = resp.read(rx); err != nil {
		return nil, err
	}
	http = time.Now().Sub(st)

	resp.Dns = dns
	resp.Tcp = tcp
	resp.Tls = ttls
	resp.Http = http
	resp.E2e = time.Now().Sub(start)
	return resp, nil
}

func (c *Client) setDeadline(conn net.Conn) {
	dl := time.Now().Add(c.Timeout)
	conn.SetDeadline(dl)
}

func (c *Client) setReadDeadline(conn net.Conn) {
	dl := time.Now().Add(c.Timeout)
	conn.SetReadDeadline(dl)
}

func (c *Client) setWriteDeadline(conn net.Conn) {
	dl := time.Now().Add(c.Timeout)
	conn.SetWriteDeadline(dl)
}

func (c *Client) resolve(host string, ctx context.Context) (net.IP, error) {
	r := &c.resolv
	ips, err := r.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil, fmt.Errorf("http: %s: %w", host, err)
	}

	// pick a random IP addr
	i := rand.IntN(len(ips))
	return ips[i], nil
}

func (r *Response) read(rd *connCloser) error {
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
	// XXX Reading body is convoluted --
	//	if chunked-encoding:
	//	    read_chunked()
	//	else:
	//	    switch content-length:
	//		case -1: read_chunked()
	//		case >= 0: read_simple_stream()
	//		default: read_till_eof()
	if has(r.Headers, "Transfer-Encoding", "chunked") {
		r.Body = NewChunkedStreamReader(rd)
	} else {
		r.Body = rd
	}
	return nil
}

func has(h Header, key, needle string) bool {
	stack := h.Values(key)
	for _, s := range stack {
		if s == needle {
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
